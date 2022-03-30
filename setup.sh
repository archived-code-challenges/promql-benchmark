#!/bin/bash

# This scripts runs under the assumption that the containers 'timescaledb' and 'promscale' and are
# running or not running at all, as both need to be started using the same postgres password.

PROJECTPATH=$(pwd)
PSQLPASSWORD=$(echo $RANDOM | md5sum | head -c 10; echo)

# Check if docker is running, error otherwise
if ! docker info > /dev/null 2>&1; then
  echo "This script uses docker, and it isn't running - please start docker and try again!"
  exit 1
fi

# Download the 'real-dataset.sz' if the file doesn't exist
if [ ! -f $PROJECTPATH/real-dataset.sz ]
then
    wget https://github.com/timescale/promscale/blob/master/pkg/tests/testdata/real-dataset.sz </dev/null &>/dev/null &
fi

# Start the 'promscale' timescaledb if not running
if [ "$( docker container inspect -f '{{.State.Status}}' timescaledb )" != "running" ]; then
    echo "Starting timescaledb..."
    timescaledb=$(docker run --name timescaledb -e POSTGRES_PASSWORD=$PSQLPASSWORD -d -p 5432:5432 --network promscale timescaledev/promscale-extension:latest-ts2-pg13 postgres -csynchronous_commit=off)
    sleep 2
fi

# Start the 'promscale' container if not running
if [ "$( docker container inspect -f '{{.State.Status}}' promscale )" != "running" ]; then
    echo "Starting promscale..."
    promscale=$(docker run --name promscale -d -p 9201:9201 --network promscale timescale/promscale:latest -db-password=$PSQLPASSWORD -db-port=5432 -db-name=postgres -db-host=timescaledb -db-ssl-mode=allow)
    sleep 2
fi

# List all needed container names
declare -a arr=("timescaledb" "promscale")

# Check containers are running
for i in "${arr[@]}"
do
   containerName=`docker ps -q -f name="$i"`
   if [ ! -n "$containerName" ]; then
    echo "$i container failed to run"
    exit 1
fi
done


if [[ ! -z "$timescaledb" ]] && [[ ! -z "$promscale" ]]; then
    echo "Postgres password set to '$PSQLPASSWORD'"
fi

# Wait until real-dataset.sz file exists
until [ -f $PROJECTPATH/real-dataset.sz ]; do
     sleep 2
done

# Ingest the sample data into Promscale and remove the binary
INGEST=$(curl -v -H "Content-Type: application/x-protobuf" -H "Content-Encoding: snappy" -H "X-Prometheus-Remote-Write-Version: 0.1.0" --data-binary "@real-dataset.sz" "http://localhost:9201/write" 2>&1 | grep "uploaded and fine")
rm -rf $PROJECTPATH/real-dataset.sz

if [[ ! -z "$INGEST" ]]; then
    echo "Setup finished: OK"
else
    echo "Setup finished: ERROR"
fi
