package main

import (
	"github.com/catalystsquad/app-utils-go/errorutils"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) (err error) {
		// catch any panics and log a nice stack trace, pulumi seems to panic a lot.
		defer func() {
			// get the recovered err in case we panicked
			recoveredErr := errorutils.RecoverErr(recover())
			// log the recovered err if there is one
			errorutils.LogOnErr(nil, "pulumi panicked", recoveredErr)
			// if there was no err from run, but there is an error from panic, return the error from the panic
			if err == nil && recoveredErr != nil {
				err = recoveredErr
			}
		}()
		return
	})
}
