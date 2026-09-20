// Package grammar is the one machine-readable description of the living
// authoring grammar: the resources a Saga records, the legal relation endpoint
// matrix, and the command shapes that write them. `change-saga spec --json`
// publishes it and `change-saga status --json` builds every next-action
// command from it, so an advertised shape and a suggested shape cannot drift.
//
// The package is a leaf. It performs no I/O and imports nothing from the
// loaders, which lets both the CLI and the next-action derivation depend on it.
package grammar

import (
	"fmt"
	"sort"
	"strings"
)

// Status says whether a command shape is callable by this binary. A planned
// shape is part of the frozen contract but its writer has not landed yet; it
// is published so an authoring agent can see the whole loop, and every action
// that uses one says so instead of pretending the command exists.
type Status string

const (
	StatusImplemented Status = "implemented"
	StatusPlanned     Status = "planned"
)

// Flag is one named argument of a command shape.
type Flag struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Required    bool   `json:"required"`
	Repeatable  bool   `json:"repeatable"`
	Description string `json:"description"`
}

// Command is one command shape. Usage is the exact one-line synopsis; Flags is
// the structured form an agent fills in.
type Command struct {
	Name        string   `json:"name"`
	Usage       string   `json:"usage"`
	Status      Status   `json:"status"`
	Mutates     bool     `json:"mutates"`
	Writes      []string `json:"writes"`
	Flags       []Flag   `json:"flags"`
	Positionals []string `json:"positionals"`
	Summary     string   `json:"summary"`
}

// Flag returns one declared flag.
func (command Command) Flag(name string) (Flag, bool) {
	for _, flag := range command.Flags {
		if flag.Name == name {
			return flag, true
		}
	}
	return Flag{}, false
}

// Argument is one flag value in an invocation. Value is empty when the author
// must supply it; Placeholder then names what is expected.
type Argument struct {
	Flag        string `json:"flag"`
	Value       string `json:"value,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// Invocation is a concrete command shape: the grammar entry plus every value
// the status projection already knows. Inputs lists what the author still has
// to decide or write; Argv renders the whole shape with placeholders so it can
// be shown, but an agent should fill Inputs rather than parse Argv.
type Invocation struct {
	Command   string     `json:"command"`
	Status    Status     `json:"status"`
	Usage     string     `json:"usage"`
	Arguments []Argument `json:"arguments"`
	Inputs    []Argument `json:"inputs"`
	Argv      []string   `json:"argv"`

	sagaPath string
}

// Value is a known flag value supplied to Invoke. Repeated names are kept in
// order for repeatable flags.
type Value struct {
	Flag  string
	Value string
}

// V is a short constructor for Value.
func V(flag, value string) Value { return Value{Flag: flag, Value: value} }

// Commands returns every command shape in stable name order.
func Commands() []Command {
	result := make([]Command, 0, len(commands))
	for _, command := range commands {
		copied := command
		copied.Flags = append([]Flag(nil), command.Flags...)
		copied.Writes = append([]string{}, command.Writes...)
		copied.Positionals = append([]string{}, command.Positionals...)
		result = append(result, copied)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// Lookup returns one command shape by its full name, for example "relation add".
func Lookup(name string) (Command, bool) {
	for _, command := range commands {
		if command.Name == name {
			return command, true
		}
	}
	return Command{}, false
}

// Invoke builds a concrete shape for the named command. It refuses a command
// or flag the grammar does not declare, so a caller can never emit a shape the
// published spec does not contain. Required flags without a known value become
// Inputs, in declaration order.
func Invoke(name, sagaPath string, values ...Value) (Invocation, error) {
	command, ok := Lookup(name)
	if !ok {
		return Invocation{}, fmt.Errorf("grammar does not declare command %q", name)
	}
	known := map[string]bool{}
	for _, value := range values {
		flag, declared := command.Flag(value.Flag)
		if !declared {
			return Invocation{}, fmt.Errorf("command %q does not declare --%s", name, value.Flag)
		}
		if known[value.Flag] && !flag.Repeatable {
			return Invocation{}, fmt.Errorf("command %q flag --%s is not repeatable", name, value.Flag)
		}
		known[value.Flag] = true
	}
	invocation := Invocation{Command: command.Name, Status: command.Status, Usage: command.Usage, Arguments: []Argument{}, Inputs: []Argument{}, sagaPath: sagaPath}
	invocation.Argv = append([]string{"change-saga"}, strings.Fields(command.Name)...)
	for _, flag := range command.Flags {
		supplied := false
		for _, value := range values {
			if value.Flag != flag.Name {
				continue
			}
			supplied = true
			if value.Value == "" {
				invocation.Inputs = append(invocation.Inputs, Argument{Flag: flag.Name, Placeholder: placeholder(flag)})
				invocation.Argv = append(invocation.Argv, "--"+flag.Name, placeholder(flag))
				continue
			}
			invocation.Arguments = append(invocation.Arguments, Argument{Flag: flag.Name, Value: value.Value})
			invocation.Argv = append(invocation.Argv, "--"+flag.Name)
			if flag.Value != "" {
				invocation.Argv = append(invocation.Argv, value.Value)
			}
		}
		if !supplied && flag.Required {
			invocation.Inputs = append(invocation.Inputs, Argument{Flag: flag.Name, Placeholder: placeholder(flag)})
			invocation.Argv = append(invocation.Argv, "--"+flag.Name, placeholder(flag))
		}
	}
	for _, positional := range command.Positionals {
		if positional == "<saga>" && sagaPath != "" {
			invocation.Argv = append(invocation.Argv, sagaPath)
			continue
		}
		invocation.Argv = append(invocation.Argv, positional)
	}
	return invocation, nil
}

// With returns the invocation with one more known flag value. It changes
// nothing when the command does not declare the flag or the flag already has
// a known value, so a caller can supply context such as the feature an action
// concerns to every shape it suggests.
func (invocation Invocation) With(flag, value string) Invocation {
	command, ok := Lookup(invocation.Command)
	if !ok || value == "" {
		return invocation
	}
	if _, declared := command.Flag(flag); !declared {
		return invocation
	}
	values := []Value{}
	for _, argument := range invocation.Arguments {
		if argument.Flag == flag {
			return invocation
		}
		values = append(values, V(argument.Flag, argument.Value))
	}
	for _, input := range invocation.Inputs {
		if input.Flag != flag {
			values = append(values, V(input.Flag, ""))
		}
	}
	values = append(values, V(flag, value))
	updated, err := Invoke(invocation.Command, invocation.sagaPath, values...)
	if err != nil {
		return invocation
	}
	return updated
}

// MustInvoke is Invoke for shapes fixed at compile time. The grammar tests
// exercise every caller's shapes, so a panic here is a programming error.
func MustInvoke(name, sagaPath string, values ...Value) Invocation {
	invocation, err := Invoke(name, sagaPath, values...)
	if err != nil {
		panic(err)
	}
	return invocation
}

func placeholder(flag Flag) string {
	if flag.Value == "" {
		return "true"
	}
	return flag.Value
}
