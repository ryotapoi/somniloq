package main

import "strings"

type stringListFlag []string

func (v *stringListFlag) String() string {
	return strings.Join(*v, ",")
}

func (v *stringListFlag) Set(value string) error {
	*v = append(*v, value)
	return nil
}
