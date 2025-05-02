#!/bin/bash

script_dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
src_dir=$script_dir/..

export DEBUG=1
export LSDC2_HOME=
export LSDC2_UID=1000
export LSDC2_GID=1000
export LSDC2_SNIFF_IFACE=eth0
export LSDC2_SNIFF_FILTER="tcp port 443"
export LSDC2_QUEUE_URL=
export LSDC2_PERSIST_FILES="scripts;README.md"
export LSDC2_BUCKET=munpri
export LSDC2_SERVER=testpilot
export LSDC2_ZIP=
export LSDC2_ZIPFROM=$src_dir
export LSDC2_CLOUDWATCH_LOG_GROUP=
export LSDC2_SNIFF_TIMEOUT=
export LSDC2_SNIFF_DELAY=
export LSDC2_EMPTY_TIMEOUT=
export LSDC2_SCAN_STDERR=true
export LSDC2_SCAN_STDOUT=true
export LSDC2_WAKEUP_SENTINEL="0 CET"
export LSDC2_LOG_SCANS=true
export LSDC2_LOG_FILTER="5 CET"
export LSDC2_LOW_MEMORY_WARNING_MB=15000
export PANIC_ON_SOCKET_ERROR=false
export DISABLE_SHUTDOWN_CALLS=true

$src_dir/lsdc2-pilot bash -c 'while true; do echo "Line written at $(date)"; sleep 1; done'
