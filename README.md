# External Initiator

Initiate Chainlink job runs from external sources.

## Installation

`go install`

## Configuration

### Environment variables

| Key                                | Description                                                                                | Example                                                            |
| ---------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------ |
| `EI_PORT`                          | The port for the EI API to listen on                                                       | `8080`                                                             |
| `EI_DATABASEURL`                   | Postgres connection URL                                                                    | `postgresql://user:pass@localhost:5432/ei`                         |
| `EI_CHAINLINKURL`                  | The URL of the Chainlink Core service                                                      | `http://localhost:6688`                                            |
| `EI_IC_ACCESSKEY`                  | The Chainlink access key, used for traffic flowing from this service to Chainlink          | `0b7d4a293bff4baf8de852bfa1f1f78a`                                 |
| `EI_IC_SECRET`                     | The Chainlink secret, used for traffic flowing from this service to Chainlink              | `h23MjHx17UJKBf3b0MWNI2P/UPh3c3O7/j8ivKCBhvcWH3H+xso4Gehny/lgpAht` |
| `EI_CI_ACCESSKEY`                  | The External Initiator access key, used for traffic flowing from Chainlink to this service | `0b7d4a293bff4baf8de852bfa1f1f78a`                                 |
| `EI_CI_SECRET`                     | The External Initiator secret, used for traffic flowing from Chainlink to this service     | `h23MjHx17UJKBf3b0MWNI2P/UPh3c3O7/j8ivKCBhvcWH3H+xso4Gehny/lgpAht` |
| `EI_KEEPER_ETH_ENDPOINT`           | The wss ethereum endpoint to use                                                           | `wss://infura.io/ws/v3/<your key>`                                 |
| `EI_KEEPER_REGISTRY_SYNC_INTERVAL` | The interval at which the keeper registry is synced                                        | `30s`                                                              |

## Build

Build the binary
```
go build -o keeper-external-initiator .
```

Build the docker image
```
docker build -t keeper-external-initiator:latest .
```

## Usage
(1) Create db

(2) Set the environment variables as described above

(3) Run
```bash
./keeper-external-initiator # local binary with env vars already set
# or
docker run --env-file ./path/to/env/vars keeper-external-initiator # docker
```

## Help Options
```
$ ./keeper-external-initiator --help
Monitors external blockchains and relays events to Chainlink node. ENV variables can be set by prefixing flag with EI_: EI_ACCESSKEY

Usage:
  external-initiator [flags]

Flags:
  --chainlinkurl string                      The URL of the Chainlink Core Service (default "localhost:6688")
  --ci_accesskey string                      The External Initiator access key, used for traffic flowing from Chainlink to this Service
  --ci_secret string                         The External Initiator secret, used for traffic flowing from Chainlink to this Service
  --cl_retry_attempts uint                   The maximum number of attempts that will be made for job run triggers (default 3)
  --cl_retry_delay duration                  The delay between attempts for job run triggers (default 1s)
  --cl_timeout duration                      The timeout for job run triggers to the Chainlink node (default 5s)
  --databaseurl string                       DatabaseURL configures the URL for external initiator to connect to
  -h, --help                                 Help for keeper-external-initiator
  --ic_accesskey string                      The Chainlink access key, used for traffic flowing from this Service to Chainlink
  --ic_secret string                         The Chainlink secret, used for traffic flowing from this Service to Chainlink
  --keeper_eth_endpoint string               The ethereum endpoint to use for keeper jobs
  --keeper_registry_sync_interval duration   The ethereum endpoint to use for keeper jobs (default 5m0s)
  --port int                                 The port for the EI API to listen on (default 8080)
```

## Adding to Chainlink

In order to use external initiators in your Chainlink node, first enable the following config in your Chainlink node's environment:

```
FEATURE_EXTERNAL_INITIATORS=true
```

This unlocks the ability to run `chainlink initiators` commands. To add an initiator run:

```bash
chainlink initiators create NAME URL
```

Where NAME is the name that you can to assign the initiator (ex: chain-init), and URL is the URL of the `/jobs` endpoint of this external-initiator service (ex: http://localhost:8080/jobs).

Once created, the output will provide the authentication necessary to add to the [environment](#environment-variables) of this external-initiator service.

Once the initiator is created, you will be able to add jobs to your Chainlink node with the type of external, and the name in the param with the name that you assigned the initiator.

### Testing

Run the entire test suite
```
go test ./...
```

