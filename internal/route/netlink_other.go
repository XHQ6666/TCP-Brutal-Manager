//go:build !linux

package route

import (
	"context"
	"fmt"
)

type Native struct{}

func (Native) Read(context.Context) ([]Route, error)             { return nil, fmt.Errorf("Linux required") }
func (Native) Change(context.Context, Route, string, bool) error { return fmt.Errorf("Linux required") }
func (Native) Watch(context.Context, chan<- struct{}) error      { return fmt.Errorf("Linux required") }
