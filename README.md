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

    ./lsdc2-pilot start-server.sh -port 2456

The above command will:
1. Fetch the key `valheim-1` in the `lsdc2cdkstack-savegame...` bucket
2. Extract the archive under `/lsdc2/savedir`
3. Start the process `start-server.sh -port 2456`

While `start-server.sh` is running, the pilot monitor the following:
* Sniff packets with the [BPF filter](https://www.tcpdump.org/manpages/pcap-filter.7.html) `udp dst port 2456`
* Check for AWS SPOT termination notices (for EC2 Spot and Fargate)
* Check 
* Trap INT and TERM signals

* Signal the process after a timeout without packets received
* Archive the files `valheim.db` and `valheim.fwl` in the S3 bucket
