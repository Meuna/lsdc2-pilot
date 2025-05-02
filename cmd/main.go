package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meuna/lsdc2-pilot/internal"

	"go.uber.org/zap"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	var logger *zap.Logger
	if os.Getenv("DEBUG") != "" {
		logger, _ = zap.NewDevelopment()
	} else {
		logger, _ = zap.NewProduction()
	}
	defer logger.Sync()

	logger.Info("Running lsdc2-pilot", zap.String("version", Version), zap.String("Commit", Commit), zap.String("BuildDate", BuildDate))

	// Initialise pilot from command line and env
	pilot := internal.NewPilot(logger, os.Args[1:])
	logger.Debug("pilot initialised", zap.Any("pilot", pilot))

	// Setup CloudWatch logger if running in EC2
	if pilot.InEc2Instance {
		newLogger, err := pilot.NewEc2CloudWatchTeeLogger(logger)
		if err != nil {
			logger.Error("error in SetupEc2Monitoring", zap.Error(err))
			pilot.NotifyBackend("error", "Failure setting EC2 monitoring")
		} else {
			logger = newLogger
			defer logger.Sync()
		}
	}

	// Prepare BPF to filter on incomming IP4 packes
	pilot.DetectIfaceAndAddHostFilter()

	// Start the process
	pilot.StartProcess()

	// Start monitoring channels
	pollingC := make(chan bool)
	terminationCheckTicker := time.NewTicker(pilot.TerminationCheckInterval)
	lowMemoryCheckTicker := time.NewTicker(pilot.TerminationCheckInterval)
	sniffTicker := time.NewTicker(pilot.SniffInterval)
	emptyTicker := time.NewTicker(pilot.EmptyTimeout)

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGTERM, syscall.SIGINT)

	defer func() {
		terminationCheckTicker.Stop()
		lowMemoryCheckTicker.Stop()
		sniffTicker.Stop()
		emptyTicker.Stop()
		pilot.StopProcess()
	}()

	if !pilot.InEc2Instance {
		terminationCheckTicker.Stop()
	}

	if pilot.LowMemoryWarningThresholdMiB == 0 && pilot.LowMemorySignalThresholdMiB == 0 {
		lowMemoryCheckTicker.Stop()
	}

	logger.Info("start monitoring network and signals")
	for {
		select {
		case packetFound := <-pollingC:
			if packetFound {
				logger.Debug("network activity detected")
				emptyTicker.Reset(pilot.EmptyTimeout)
			}
		case <-sniffTicker.C:
			go func() {
				pollingC <- pilot.PollProcessPackets()
			}()
		case <-emptyTicker.C:
			logger.Info("server empty for too long")
			pilot.NotifyBackend("info", "Server empty. Terminating instance.")
			return
		case <-terminationCheckTicker.C:
			logger.Debug("checking SPOT termination")
			terminationNotified, err := internal.SpotTerminationIsNotified()
			if err != nil {
				logger.Error("error getting termination notification", zap.Error(err))
				pilot.NotifyBackend("error", "Error worth checking in the EC2 instance")
			}
			if terminationNotified {
				logger.Info("spot termination detected")
				pilot.NotifyBackend("warning", "SPOT termination detected. Terminating instance.")
				return
			}
		case <-lowMemoryCheckTicker.C:
			logger.Debug("checking low memory")
			freeMemoryMib, err := internal.GetFreeMemoryMiB()
			if err != nil {
				logger.Error("error getting free memory", zap.Error(err))
				pilot.NotifyBackend("error", "Error checking free memory")
			}
			if freeMemoryMib < pilot.LowMemorySignalThresholdMiB {
				logger.Warn("low memory signal", zap.Int64("freeMemory", freeMemoryMib))
				pilot.NotifyBackend("warning", fmt.Sprintf("Memory limit breached (%d MiB). Terminating instance.", freeMemoryMib))
				return
			} else if freeMemoryMib < pilot.LowMemoryWarningThresholdMiB {
				logger.Warn("low memory warning", zap.Int64("freeMemory", freeMemoryMib))
				pilot.NotifyBackend("warning", fmt.Sprintf("Low memory warning (%d MiB)", freeMemoryMib))
			}
		case <-sigC:
			logger.Info("received signal")
			pilot.NotifyBackend("warning", "Signal received. Terminating instance.")
			return
		}
	}
}
