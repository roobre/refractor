# 🪞 Refractor

Refractor is linux mirror load-balancer, which parallelizes requests between an extremely dynamic pool of mirrors. Mirrors in the pool are constantly monitored for throughput, and slowest mirrors are continuously rotated out of the pool and replaced by new ones.

## Working principle

The core of Refractor is a pool of workers, to which HTTP requests are routed. A worker mapped to a particular mirror performs the request to said mirror and proxies the response to the user.

Before considering a request, workers look how well they are performing compared to their peers. If they are on the bottom two positions of the ranking, they will resign and get out of the pool. The pool will automatically add a worker for a different mirror to compensate.

This way, the pool of active mirrors is constantly rotating slow mirrors out of the pool, based on their current performance. This eliminates the need of continuously benchmarking mirrors, and avoids having to assume that mirrors' bandwidth is constant in time.

## Intended usage

Refractor is intended to be run either locally, or in a local network where linux machines reside. This is because Refractor drops mirrors aggressively based on mirror-to-client throughput, and therefore it will not be effective if clients with different effective throughput to the host running Refractor connect to it. Moreover, for this same reason, bad actors could deliberately simulate bad latencies and kick good mirrors out of the pool, degrading service quality for others.

## Providers

Refractor is designed to be distribution-agnostic, as long as a Provider that can fetch a mirror and feed it to the pool is implemented. Refractor automatically sorts the pool of mirrors automatically by the throughput they provide as request come by. This means that providers do not need to sort or benchmark mirrors before supplying them to the pool.

It is recommended, however, for providers to apply coarse-grain filter such as physical location, as doing so will allow the pool to stabilize faster.

For the moment, the following providers exist:

### Arch Linux (`archlinux`)

The Arch Linux provider feeds mirrors from `https://archlinux.org/mirrors/status/json/`, after applying some user-defined filters. For now, filtering by country and by score is allowed.

```yaml
workers: 8
goodThroughputMiBs: 10

provider:
  archlinux:
    maxScore: 5
    countries:
      - ES
      - IT
      - FR
      - PT
```

### Command (`command`)

The Command provider allows to feed to the pool mirror URLs obtained from running an user-defined command. This should help as an stop-gap for supporting distros without coding providers from them.

> ⚠️ Refractor rotates mirrors from the pool very aggressively, which means the specified command will be called multiple times and very often. Please make sure this command is not hammering any public API without appropriate caching.

```yaml
workers: 8
goodThroughputMiBs: 10

provider:
  command:
    #shell: /bin/bash # Defaults to $SHELL, then to /bin/sh
    command: |
      cat <<EOF | sort -R | head -n 1
        http://foo.bar
        http://example.local
        https://another.mirror
      EOF
```

The specified command is expected to return a single line containing the mirror URL. If more than one line is printed, Refractor will emit a warning and ignore the rest. Refractor will echo the command's standard error as log lines with `warning` level.

### Implement your own!

Providers are very easy to implement in-code, as they only need to be able to retrieve a random mirror from a list.

```go
type Provider interface {
	Mirror() (string, error)
}
```

As an example, the Arch Linux mirror provider retrieves the list of mirrors from `https://archlinux.org/mirrors/status/json/`, applies some user-defined country and score settings, and returns a random mirror from the resulting list.

Implementing providers in code is encouraged as it provides maximum flexibility to control caching and configuration options. PRs are welcome!

## Advanced features

- **Average window**: Only the last few throughput measurments are averaged when checking how a mirror is performing. This allow rotating out mirrors that start to behave poorly even if they have been very performant in the past.
- **Absolutely good throughput**: Mirrors that perform better than `goodThroughputMiBs` will not be rotated from the pool, even if they are the least performant.
- **Request peeking**: Refractor will "peek" the first few megs (`peekSizeMiBs`) from the connection to a mirror before passing the response to the client. If this peek operation takes too long (`peekTimeout`), the request will be requeued to a different mirror.

## Project status

Currently this project is in _indev_ stage, meaning it is currently being tested on my local machine. Contributions are welcome, especially if you get in touch with me by opening an issue or sending me an email.
