package providers

type Provider interface {
	Mirror() (string, error)
}
