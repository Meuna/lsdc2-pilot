# lsdc2-pilot

Standalone wrapper for linux game server for an LSD2 stack. Its main function is
to retrieve/persist game files in S3, check for Spot termination notices, and
check for network traffic.

## Usage

    export LSDC2_HOME=/lsdc2
    export LSDC2_UID=2000
    export LSDC2_GID=2000
    export LSDC2_SNIFF_FILTER="udp dst port 2456"
    export LSDC2_QUEUE_URL=https://sqs.xxxx.amazonaws.com/xxxx/Lsdc2CdkStack-discordBotQueue...
    export LSDC2_PERSIST_FILES="valheim.db;valheim.fwl"
    export LSDC2_BUCKET=lsdc2cdkstack-savegame...
    export LSDC2_SERVER=valheim-1
    export LSDC2_ZIPFROM=/lsdc2/savedir
    export LSDC2_CLOUDWATCH_LOG_GROUP=Lsdc2CdkStack-...
    export LSDC2_SCAN_STDERR=true
    export LSDC2_SCAN_STDOUT=true
    export LSDC2_WAKEUP_SENTINEL="Game server connected"
    export LSDC2_LOG_SCANS=true
    export LSDC2_LOW_MEMORY_WARNING_MB=1024
    export LSDC2_LOW_MEMORY_SIGNAL_MB=512

    ./lsdc2-pilot start-server.sh -port 2456

The above command will:
1. fetch the key `valheim-1` in the `lsdc2cdkstack-savegame...` bucket,
2. extract the archive under `/lsdc2/savedir`,
3. and start the process `start-server.sh -port 2456` from the `/lsdc2` working
   directory, with the `2000:2000` uid:gid.

While `start-server.sh` is running, the pilot monitor the following:
* Sniff packets with the [BPF filter](https://www.tcpdump.org/manpages/pcap-filter.7.html) `udp dst port 2456`.
* Check for AWS SPOT termination notices (for EC2 Spot and Fargate).
* Check free memory reading `/proc/meminfo`.
* Trap INT and TERM signals.

After a timeout without traffic, a Spot termination notice, a low memory breach
or a signal:
1. the process is signaled with TERM,
2. and the files `valheim.db` and `valheim.fwl` are archived to the key `valheim-1`
   in the `lsdc2cdkstack-savegame...` bucket.

Additionally, the pilot is configured to scan output and error standard streams:
* With `LSDC2_LOG_SCANS=true`, the streams are sent to the pilot standard output
  and the CloudWatch Log group `Lsdc2CdkStack-...`.
* When the sentinel string `"Game server connected"` is found in the stream, the
  LSDC2 stack is notified that the game server is ready.

## Environment reference

| **Variable**                       | **Default** | **Description**
| ---------------------------------- | ----------- | ---------------
| `LSDC2_HOME`                       |             | Working directory of the forked process.
| `LSDC2_UID`                        |             | UID of the forked process
| `LSDC2_GID`                        |             | GID of the forked process.
| `LSDC2_QUEUE_URL`                  |             | URL of the LSDC2 stack SQS notification queue.
| `LSDC2_PERSIST_FILES`              |             | Semicolon separated list of file path to be persisted to S3.
| `LSDC2_BUCKET`                     |             | S3 bucket on which server files are persisted.
| `LSDC2_SERVER`                     |             | Name of the server. Used as the S3 key.
| `LSDC2_ZIP`                        |             | Can be set to a falsy value for niche cases where zipping is undesired.
| `LSDC2_ZIPFROM`                    |             | Root folder for archiving and unarchiving.
| `LSDC2_LOG_GROUP`                  |             | The CloudWatch Log group traces are sent to.
| `LSDC2_LOG_FLUSH_INTERVAL`         | 5s          | Time interval between traces flushing to CloudWatch Log.
| `LSDC2_TERMINATION_CHECK_INTERVAL` | 10s         | Time interval between EC2 Spot termination check.
| `LSDC2_SIGNAL_GRACE_DELAY`         | 20s         | Grace delay before the forked process is signaled to terminate.
| `LSDC2_SNIFF_FILTER`               |             | BPF filter used to sniff traffic.
| `LSDC2_SNIFF_TIMEOUT`              | 1s          | Traffic sniff timeout.
| `LSDC2_SNIFF_INTERVAL`             | 10s         | Time interval between traffic sniffs.
| `LSDC2_EMPTY_TIMEOUT`              | 5m          | Terminate the process after this timeout without traffic.
| `LSDC2_SCAN_STDERR`                | false       | If truthy, standard error stream is scanned.
| `LSDC2_SCAN_STDOUT`                | false       | If truthy, standard output stream is scanned.
| `LSDC2_WAKEUP_SENTINEL`            |             | Sentinel string searched in scanned streams to detect that the game server is ready.
| `LSDC2_LOG_SCANS`                  | false       | If truthy, the scanned streams are traced.
| `LSDC2_LOG_FILTER`                 |             | Semicolon separated list of keywords used to limit the tracing of scanned streams.
| `LSDC2_LOW_MEMORY_WARNING_MB`      | 0           | If memory breach this threshold, send a notification to the LSDC2 queue.
| `LSDC2_LOW_MEMORY_SIGNAL_MB`       | 0           | If memory breach this threshold, terminate the process.
| `LSDC2_LOW_MEMORY_CHECK_INTERVAL`  | 5s          | Time interval between memory check.
| `PANIC_ON_SOCKET_ERROR`            | true        | If falsy, disable panic on socket related errors (used for debugging).
| `DISABLE_SHUTDOWN_CALLS`           | true        | If falsy, disable issuance of shutdown command during termination (used for debugging).
| `DEBUG`                            |             | If non-empty, set trace level to Development.
