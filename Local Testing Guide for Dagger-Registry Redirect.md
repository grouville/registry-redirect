# DAGGER-REGISTRY REDIRECT LOCAL TESTING

This guide provides the steps to test the Dagger-Registry Redirect application locally, focusing on recent improvements to the syslog.

## Procedure

Follow the steps below on different terminal instances:

### Setting up Vector Configuration Locally

1. Install [Vector]((https://vector.dev/docs/setup/installation/)).
2. Copy the following configuration and run it with Vector: vector --config vector.toml

```toml file=vector.toml
[sources.my_syslog_source]
  type = "syslog"
  mode = "udp"
  address = "0.0.0.0:514"

[transforms.my_filter]
  type = "filter"
  inputs = ["my_syslog_source"]
  condition = '''
message = string!(.message)
contains(message, "dagger-registry-2023-01-23")
'''

[sinks.my_console_sink]
  type = "console"
  inputs = ["my_filter"]
  target = "stdout"
  encoding.codec = "json"
```

### Modifying the Hosts File

Add the following line to your /etc/hosts file:

```shell
127.0.0.1 toto.localhost 
```

### Setting up Caddy Reverse Proxy

1. Install [Caddy](https://caddyserver.com/docs/install)
2. Run the following command to setup a reverse proxy:

```shell
caddy reverse-proxy --from toto.localhost --to http://localhost:8080
```

### Running the Main Application

With the environment set up, run the Dagger-Registry Redirect application:

```shell
go run . -repo dagger
```

You should see the logs locally and in the Vector instance.

To check the next logs, run the following Docker command (assuming the [equinix](https://github.com/orgs/dagger/packages/container/package/equinix-demo-day-2023) package exists). This command does not interfere with production as the registry-redirect is running locally and is separate from our Vector configurations.

```shell
docker pull toto.localhost/equinix-demo-day-2023:latest
```