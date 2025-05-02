package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Pilot struct {
	logger       *zap.Logger
	cl           []string
	cmd          *exec.Cmd
	sigWith      os.Signal
	processStart time.Time
	iface        string

	Home string `env:"LSDC2_HOME"`
	Uid  int    `env:"LSDC2_UID"`
	Gid  int    `env:"LSDC2_GID"`

	QueueUrl     string   `env:"LSDC2_QUEUE_URL"`
	PersistFiles []string `env:"LSDC2_PERSIST_FILES" envSeparator:";"`
	Bucket       string   `env:"LSDC2_BUCKET"`
	Server       string   `env:"LSDC2_SERVER"`
	Zip          bool     `env:"LSDC2_ZIP"`
	ZipFrom      string   `env:"LSDC2_ZIPFROM"`

	InEc2Instance            bool
	CloudWatchLogGroup       string        `env:"LSDC2_LOG_GROUP"`
	CloudWatchFlushInterval  time.Duration `env:"LSDC2_LOG_FLUSH_INTERVAL" envDefault:"5s"`
	TerminationCheckInterval time.Duration `env:"LSDC2_TERMINATION_CHECK_INTERVAL" envDefault:"10s"`
	SignalGraceDelay         time.Duration `env:"LSDC2_SIGNAL_GRACE_DELAY" envDefault:"20s"`
	SniffFilter              string        `env:"LSDC2_SNIFF_FILTER"`
	SniffTimeout             time.Duration `env:"LSDC2_SNIFF_TIMEOUT" envDefault:"1s"`
	SniffInterval            time.Duration `env:"LSDC2_SNIFF_INTERVAL" envDefault:"10s"`
	EmptyTimeout             time.Duration `env:"LSDC2_EMPTY_TIMEOUT" envDefault:"5m"`

	ScanStderr     bool     `env:"LSDC2_SCAN_STDERR" envDefault:"false"`
	ScanStdout     bool     `env:"LSDC2_SCAN_STDOUT" envDefault:"false"`
	WakeupSentinel string   `env:"LSDC2_WAKEUP_SENTINEL"`
	LogScans       bool     `env:"LSDC2_LOG_SCANS" envDefault:"false"`
	LogFilter      []string `env:"LSDC2_LOG_FILTER" envSeparator:";"`

	LowMemoryWarningThresholdMiB int64         `env:"LSDC2_LOW_MEMORY_WARNING_MB" envDefault:"0"`
	LowMemorySignalThresholdMiB  int64         `env:"LSDC2_LOW_MEMORY_SIGNAL_MB" envDefault:"0"`
	LowMemoryCheckInterval       time.Duration `env:"LSDC2_LOW_MEMORY_CHECK_INTERVAL" envDefault:"5s"`

	PanicOnSocketError   bool `env:"PANIC_ON_SOCKET_ERROR" envDefault:"true"`
	DisableShutdownCalls bool `env:"DISABLE_SHUTDOWN_CALLS" envDefault:"false"`
}

func NewPilot(logger *zap.Logger, cl []string) Pilot {
	p := Pilot{}
	var err error
	if err = env.Parse(&p); err != nil {
		panic(err)
	}

	// This is not redundant with the envDefault defined in Config
	// struct because empty env variables are not equivalent to
	// empty variables. The former makes the values 0
	if p.SniffTimeout == 0 {
		p.SniffTimeout = 1 * time.Second
	}
	if p.SniffInterval == 0 {
		p.SniffInterval = 10 * time.Second
	}
	if p.EmptyTimeout == 0 {
		p.EmptyTimeout = 5 * time.Minute
	}

	p.Zip = p.Zip || len(p.PersistFiles) > 1

	p.logger = logger
	p.cl = cl
	p.sigWith = syscall.SIGTERM
	p.InEc2Instance = AreWeRunningEc2()

	return p
}

func (p *Pilot) NewEc2CloudWatchTeeLogger(logger *zap.Logger) (*zap.Logger, error) {
	p.InEc2Instance = true
	instanceId, err := GetInstanceId()
	if err != nil {
		return nil, fmt.Errorf("GetInstanceId / %w", err)
	}
	cloudWatchCore, err := NewCloudWatchCore(
		zap.InfoLevel,
		p.CloudWatchLogGroup,
		fmt.Sprintf("ec2/%s_instance/%s", p.Server, instanceId),
		100,
		p.CloudWatchFlushInterval,
	)
	if err != nil {
		return nil, fmt.Errorf("NewCloudWatchCore / %w", err)
	}
	teeCore := zapcore.NewTee(
		cloudWatchCore,
		logger.Core(),
	)

	p.logger = zap.New(teeCore)

	return p.logger, nil
}

func (p *Pilot) DetectIfaceAndAddHostFilter() {
	if iface, ip, err := GetFirstIfaceWithIp4(); err == nil {
		p.logger.Debug("found iface", zap.String("iface", iface), zap.String("ip", ip.String()))
		filterWithDest := fmt.Sprintf("dst host %v", ip)
		if p.SniffFilter != "" {
			filterWithDest = fmt.Sprintf("(%v) and (%v)", filterWithDest, p.SniffFilter)
		}
		p.iface = iface
		p.SniffFilter = filterWithDest
	} else {
		p.logger.Debug("iface not found, using 'any'", zap.Error(err))
		p.iface = "any"
	}
	p.logger.Debug("final BPF filter", zap.String("filter", p.SniffFilter))
}

func (p *Pilot) StartProcess() {
	if len(p.PersistFiles) > 0 {
		p.logger.Info("downloading from S3")
		err := p.retrieveData()
		if err != nil {
			p.logger.Error("error in StartProcess", zap.String("culprit", "retrieveData"), zap.Error(err))
			p.NotifyBackend("error", "Savegame was not restored")
		} else {
			p.logger.Info("S3 download done !")
			p.NotifyBackend("info", "Savegame restored from S3")
		}
	}

	p.logger.Debug("cmd initialisation", zap.Strings("cl", p.cl))
	p.cmd = exec.Command(p.cl[0], p.cl[1:]...)
	scannedStreams := []io.ReadCloser{}
	if p.Home != "" {
		p.logger.Debug("set cmd working directory", zap.String("cwd", p.Home))
		p.cmd.Dir = p.Home
	}
	if (p.Uid != 0) || (p.Gid != 0) {
		p.logger.Debug("set cmd uid/gid", zap.Int("uid", p.Uid), zap.Int("gid", p.Gid))
		p.cmd.SysProcAttr = &syscall.SysProcAttr{}
		p.cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(p.Uid), Gid: uint32(p.Gid)}
	}
	if p.ScanStderr {
		p.logger.Debug("get cmd stderr stream")
		stream, err := p.cmd.StderrPipe()
		if err != nil {
			p.logger.Panic("error in StartProcess", zap.String("culprit", "StderrPipe"), zap.Error(err))
		}
		scannedStreams = append(scannedStreams, stream)
	}
	if p.ScanStdout {
		p.logger.Debug("get cmd stdout stream")
		stream, err := p.cmd.StdoutPipe()
		if err != nil {
			p.logger.Panic("error in StartProcess", zap.String("culprit", "StdoutPipe"), zap.Error(err))
		}
		scannedStreams = append(scannedStreams, stream)
	}
	p.logger.Debug("start cmd")
	if err := p.cmd.Start(); err != nil {
		p.logger.Panic("error in StartProcess", zap.String("culprit", "Start"), zap.Error(err))
	}
	if len(scannedStreams) > 0 {
		p.logger.Info("std scan enabled", zap.String("wakeupSentinel", p.WakeupSentinel), zap.Bool("logScans", p.LogScans), zap.Any("logFilter", p.LogFilter))
		p.enableStdScans(scannedStreams)
	}
	p.logger.Info("process started")
	p.processStart = time.Now()
}

func (p *Pilot) enableStdScans(streams []io.ReadCloser) {
	logChan := make(chan string, 60)
	wakeupChan := make(chan string, 60)
	for _, stream := range streams {
		scanner := bufio.NewScanner(stream)
		go func() {
			for scanner.Scan() {
				line := scanner.Text()
				line = strings.TrimSpace(line)
				if p.LogScans {
					if len(p.LogFilter) > 0 {
						for _, word := range p.LogFilter {
							if strings.Contains(line, word) {
								logChan <- line
								break
							}
						}
					} else {
						logChan <- line
					}
				}
				if p.WakeupSentinel != "" && strings.Contains(line, p.WakeupSentinel) {
					wakeupChan <- line
				}
			}
		}()
	}
	go func() {
		for line := range logChan {
			p.logger.Info(line)
		}
	}()
	go func() {
		for line := range wakeupChan {
			timeSinceStart := time.Now().Sub(p.processStart)
			p.logger.Info("sentinel found", zap.String("sentinel", line))
			p.NotifyBackend("server-ready", fmt.Sprintf("The server is ready ! (started in %.2fs)", timeSinceStart.Seconds()))
		}
	}()
}

func (p *Pilot) PollProcessPackets() bool {
	packetFound, err := PollFilteredIface(p.iface, p.SniffFilter, p.SniffTimeout)
	if err != nil {
		p.logger.Error("error polling network",
			zap.String("iface", p.iface),
			zap.String("filter", p.SniffFilter),
			zap.Error(err),
		)
		if p.PanicOnSocketError {
			p.ShutdownWhenInEc2()
			panic(err)
		}
	}
	return packetFound
}

func (p *Pilot) StopProcess() {
	// Grace delay after warning
	time.Sleep(p.SignalGraceDelay)

	// Stop the process
	p.cmd.Process.Signal(p.sigWith)
	p.cmd.Wait()

	// Small wait to sync file system
	time.Sleep(1 * time.Second)

	if len(p.PersistFiles) > 0 {
		p.logger.Info("S3 upload")
		err := p.archiveData()
		if err != nil {
			p.logger.Error("error in StopProcess", zap.String("culprit", "archiveData"), zap.Error(err))
			p.NotifyBackend("error", "Error when exporting savegame to S3")
		} else {
			p.NotifyBackend("info", "Savegame exported to S3")
		}
	}

	p.ShutdownWhenInEc2()

	p.logger.Info("goodbye !")
}

func (p *Pilot) ShutdownWhenInEc2() {
	// Clear early return if this is true
	if p.DisableShutdownCalls {
		return
	}
	if p.InEc2Instance {
		p.logger.Info("issue shutdown")
		cmd := exec.Command("shutdown", "now")

		err := cmd.Run()
		if err != nil {
			p.logger.Error("error in StopProcess", zap.String("culprit", "Run"), zap.Error(err))
			p.NotifyBackend("error", "Error worth checking in the EC2 instance")
		}
	}
}

func (p *Pilot) retrieveData() error {
	if p.Zip {
		return unzipFromS3(p.logger, p.Bucket, p.Server, p.ZipFrom, p.Uid, p.Gid)
	} else {
		return downloadFromS3(p.Bucket, p.Server, p.PersistFiles[0], p.Uid, p.Gid)
	}
}

func (p *Pilot) archiveData() error {
	if p.Zip {
		return zipToS3(p.logger, p.Bucket, p.Server, p.ZipFrom, p.PersistFiles)
	} else {
		return uploadToS3(p.Bucket, p.Server, p.PersistFiles[0])
	}
}

func (p *Pilot) NotifyBackend(action string, msg string) {
	cmd := struct {
		Api  string
		Args any
	}{
		Api: "tasknotify",
		Args: struct {
			ServerName string
			Action     string
			Message    string
		}{
			ServerName: p.Server,
			Action:     action,
			Message:    msg,
		},
	}
	bodyBytes, err := json.Marshal(cmd)
	if err != nil {
		p.logger.Error("error in NotifyBackend", zap.String("culprit", "Marshal"), zap.Error(err))
		return
	}
	err = queueMessage(p.QueueUrl, string(bodyBytes[:]))
	if err != nil {
		p.logger.Error("error in NotifyBackend", zap.String("culprit", "queueMessage"), zap.Error(err))
	}
}
