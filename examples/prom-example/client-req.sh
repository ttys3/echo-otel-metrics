#!/usr/bin/env bash

set -eou pipefail

#!/bin/sh

for (( ; ; ))
do
    traceid=$(uuidgen | tr -d '-')
    echo "traceid: ${traceid}"
    curl -H "X-B3-Traceid: ${traceid}" -H 'X-B3-Spanid: 0000000000000001' -H 'X-B3-Sampled: 1' http://localhost:1323/memory-test
   echo "infinite loops [ hit CTRL+C to stop]"
done



