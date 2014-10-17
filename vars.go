package main

import (
	"fmt"
	"regexp"
	"strings"
)

type stringSliceVar struct {
	s *[]string
}

func (f stringSliceVar) String() string {
	return strings.Join(*f.s, " + ")
}

func (f stringSliceVar) Set(v string) error {
	*f.s = append(*f.s, v)
	return nil
}

type regexSliceVar struct {
	s *[]*regexp.Regexp
}

func (f regexSliceVar) String() string {
	return fmt.Sprintf("%+v", f.s)
}

func (f regexSliceVar) Set(v string) error {
	re, err := regexp.Compile(v)
	if err != nil {
		return err
	}
	*f.s = append(*f.s, re)
	return nil
}
