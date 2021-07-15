# Docker Swarm Haproxy Ingress Controller

The aim of the project is it to create dynamic ingress rules for swarm services through labels. This allows to create new services and change the haproxy configuration without any downtime or container rebuild.

The manager service is responsible for generating a valid haproxy configuration file from the labels. The loadbalancer instances scrape the configuration periodically and reload the worker "hitless" if the content has changed.

## Synopsis

```yml
version: "3.9"

services:
    # the lb service fetches the dynamic configuration,
    # from the manager endpoint, periodically
    loadbalancer:
        image: bluebrown/swarm-haproxy-loadbalancer
        # default values for env vars are below
        environment: 
            MANAGER_ENDPOINT: http://manager:8080/
            SCRAPE_INTERVAL: '60'
            STARTUP_DELAY: '5'
        depends_on:
            - manager
        ports:
            - 3000:80 # ingress port
            - 4450:4450 # stats page

    # the manager service defines global defaults and frontend configs
    manager:
        image: bluebrown/swarm-haproxy-manager
        # default template path, this is not required
        # but can be useful when mounting a config file as volume
        command: --template /src/haproxy.cfg.template 
        volumes: 
            -  /var/run/docker.sock:/var/run/docker.sock
        ports:
            - 8080:8080
        labels:
            ingress.global: |
                spread-checks 15
            ingress.defaults: |
                timeout connect 5s
                timeout check 5s
                timeout client 2m
                timeout server 2m
            ingress.frontend.default: |
                bind *:80
        deploy:
            placement:
                constraints:
                    # needs to be on a manager node to read the services
                    - "node.role==manager"

    # each app service defines its own backend config
    # and can provide a frontend snippet for 1 or more frontend.
    # The snippet will be merged with the frontend config from the
    # manager service
    some-app:
        image: nginx
        deploy:
            replicas: 2
            labels:
                # the application port inside the container
                ingress.port: "80"
                # rules are merged with the corresponding frontend rules
                ingress.frontend.default: |
                    use_backend {{ .Name }} if { path -i -m beg /foo/ }
                # backend snippet are added to the backend created from
                # this service definition
                ingress.backend: |
                    balance roundrobin
                    option httpchk GET /
                    http-request set-path "%[path,regsub(^/foo/,/)]"

```

Currently it only works when deploying the *backend* services with swarm. The manager can be deployed with a normal container. This is because the labels for the manager are provided on container level while the backends are created from service definitions and their labels.

> Note  
> *Be careful which ports you publish in production*

## Template

The labels are parsed and passed into a haproxy.cfg template. The default template looks like the below.

```go
resolvers docker
    nameserver dns1 127.0.0.11:53
    resolve_retries 3
    timeout resolve 1s
    timeout retry   1s
    hold other      10s
    hold refused    10s
    hold nx         10s
    hold timeout    10s
    hold valid      10s
    hold obsolete   10s

global
    log          fd@2 local2
    stats socket /var/run/haproxy.pid mode 600 expose-fd listeners level user
    stats timeout 2m
    {{ .Global | indent 4 | trim }}

defaults
    log global
    mode http
    option httplog
    {{ .Defaults | indent 4 | trim }}

listen stats
    bind *:4450
    stats enable
    stats uri /
    stats refresh 15s
    stats show-legends
    stats show-node {{ println "" }}

{{- range $frontend, $config := .Frontend }}
frontend {{$frontend}}
    {{$config | indent 4 | trim }}
{{ end }}

{{- range $backend, $config := .Backend }}
backend {{$backend}}
    {{ $config.Backend | indent 4 | trim }}
    server-template {{ $backend }}- {{ $config.Replicas }} tasks.{{ $backend }}:{{ $config.Port }} resolvers docker init-addr libc,none check
{{ end }}

{{ println ""}}
```

The data types passed into the template have the following format.

```go
type ConfigData struct {
 Global   string             `json:"global,omitempty"`
 Defaults string             `json:"defaults,omitempty"`
 Frontend map[string]string  `json:"frontend,omitempty"`
 Backend  map[string]Backend `json:"backend,omitempty"`
}

type Backend struct {
 Port     string            `json:"port,omitempty"`
 Replicas uint64            `json:"replicas,omitempty"`
 Frontend map[string]string `json:"-"`
 Backend  string            `json:"backend,omitempty"`
}
```

Frontend snippets in the backend struct are executed as template and merged with the frontend config from the ConfigData struct. That is why it is not returned as json and not directly used in the template.

### Configuration

The configuration can be fetched via the root endpoint of the manager service. The raw data to populate the template is available in json format.

```shell
curl -i localhost:8080
curl -i localhost:8080/json
```

### Update the template at runtime

It is possible to update the template at runtime via post request.

```shell
curl -i -X POST localhost:8080/update --data-binary @path/to/template
```

### Example JSON response

```json
{
    "global": "spread-checks 15\n",
    "defaults": "timeout connect 5s\ntimeout check 5s\ntimeout client 2m\ntimeout server 2m\n",
    "frontend": {
        "default": "bind *:80\nuse_backend my-stack_some-app if { path -i -m beg /foo/ }\n"
    },
    "backend": {
        "my-stack_some-app": {
            "port": "80",
            "replicas": 2,
            "backend": "balance roundrobin\noption httpchk GET /\nhttp-request set-path \"%[path,regsub(^/foo/,/)]\"\n"
        }
    }
}
```

### Example Config Response

```c
resolvers docker
    nameserver dns1 127.0.0.11:53
    resolve_retries 3
    timeout resolve 1s
    timeout retry   1s
    hold other      10s
    hold refused    10s
    hold nx         10s
    hold timeout    10s
    hold valid      10s
    hold obsolete   10s

global
    log          fd@2 local2
    stats socket /var/run/haproxy.pid mode 600 expose-fd listeners level user
    stats timeout 2m
    spread-checks 15

defaults
    log global
    mode http
    option httplog
    timeout connect 5s
    timeout check 5s
    timeout client 2m
    timeout server 2m 

listen stats
    bind *:4450
    stats enable
    stats uri /
    stats refresh 15s
    stats show-legends
    stats show-node 

frontend default
    bind *:80
    use_backend my-stack_some-app if { path -i -m beg /foo/ }

backend my-stack_some-app
    balance roundrobin
    option httpchk GET /
    http-request set-path "%[path,regsub(^/foo/,/)]"
    server-template my-stack_some-app- 2 tasks.my-stack_some-app:80 resolvers docker init-addr libc,none check

```

## Local Development

If you have the repository locally, you can use the `Makefile` to run a example deployment.

```shell
make
curl -i localhost:3000 # backend app
curl -i localhost:3000/foo/ # backend foo
curl -i localhost:4450 # stats page
```
