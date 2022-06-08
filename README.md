# 🪞 Shatter

Shatter is linux mirror load-balancer, which parallelizes requests between an extremely dynamic pool of mirrors. Mirrors in the pool are constantly monitored for throughput, and slowest mirrors are continuously rotated out of the pool and replaced by new ones.

## Working principle

The core of Shatter is a pool of workers, to which HTTP requests are routed. A worker mapped to a particular mirror performs the request to said mirror and proxies the response to the user.

Before considering a request, workers look how well they are performing compared to their peers. If they are on the bottom two positions of the ranking, they will resign and get out of the pool. The pool will automatically add a worker for a different mirror to compensate.

This way, the pool of active mirrors is constantly rotating slow mirrors out of the pool, based on their current performance. This eliminates the need of continuously benchmarking mirrors, and avoids having to assume that mirrors' bandwidth is constant in time.

## Intended usage

Shatter is intended to be run either locally, or in a local network where linux machines reside. This is because Shatter drops mirrors aggressively based on mirror-to-client throughput, and therefore it will not be effective if clients with different effective throughput to the host running Shatter connect to it. Moreover, for this same reason, bad actors could deliberately simulate bad latencies and kick good mirrors out of the pool, degrading service quality for others.

## Providers

Shatter is designed to be distribution-agnostic, as long as a Provider that can fetch a list of mirrors and feed them to the pool is implemented. For the moment, providers exists for:

- Arch Linux (btw)

Providers are very easy to implement, as they only need to be able to retrieve a random mirror from a list:

```go
type Provider interface {
	Mirror() (string, error)
}
```

As an example, the Arch Linux mirror provider retrieves the list of mirrors from `https://archlinux.org/mirrors/status/json/`, applies some user-defined country and score settings, and returns a random mirror from the resulting list. 

## Project status

Currently this project is in _indev_ stage, meaning it is currently being tested on my local machine. Contributions are welcome, especially if you get in touch with me by opening an issue or sending me an email.
