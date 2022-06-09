// Package command implements a provider that feeds mirrors from a predefined shell command.
package command

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"roob.re/refractor/provider/types"
	"strings"
)

const defaultShell = "/bin/sh"

type Provider struct {
	config
}

type config struct {
	Command string `yaml:"command"`
	Shell   string `yaml:"shell"`
}

func DefaultConfig() interface{} {
	return &config{}
}

func New(conf interface{}) (types.Provider, error) {
	cmdConfig, ok := conf.(*config)
	if !ok {
		return nil, fmt.Errorf("internal error: supplied config is not of the expected type")
	}

	if cmdConfig.Command == "" {
		return nil, fmt.Errorf("invalid command %q", cmdConfig.Command)
	}

	if cmdConfig.Shell == "" {
		cmdConfig.Shell = os.Getenv("SHELL")
	}

	if cmdConfig.Shell == "" {
		cmdConfig.Shell = defaultShell
		log.Warnf("Could not figure out shell from the environment ($SHELL), using code default")
	}

	log.Infof("Using %q as a shell to run commands", cmdConfig.Shell)

	return Provider{
		config: *cmdConfig,
	}, nil
}

func (p Provider) Mirror() (string, error) {
	cmd := exec.Command(p.Shell, "-c", p.Command)

	stderr := log.NewEntry(log.StandardLogger()).WithField("command", p.Command).Writer()
	defer stderr.Close()

	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running %q: %w", p.Command, err)
	}

	outLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(outLines) > 1 {
		log.Warnf("Output of %q contains multiple lines, only the first will be used", p.Command)
	}

	return strings.TrimSpace(outLines[0]), nil
}
