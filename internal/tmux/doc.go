// Package tmux wraps the tmux CLI. Runner is the abstraction; CLIRunner is
// the production implementation that shells out to `tmux`. Tests use
// tmuxtest.MockRunner from the sibling package.
package tmux
