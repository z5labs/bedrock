// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package bedrock provides a functional, composable framework for building and running applications.
//
// The package is built around three core abstractions:
//
//   - Builder[T]: A generic interface for constructing application components with context support
//   - Runtime: An interface representing a runnable application component
//   - Runner[T]: An interface for executing application components built from Builders
//
// # Functional Composition
//
// Bedrock embraces functional programming patterns to enable flexible composition:
//
//   - Map: Transform builder outputs using pure functions
//   - Bind: Chain builders together, allowing the output of one to inform the construction of the next
//
// These combinators allow you to build complex applications from simple, reusable components.
//
// # Basic Usage
//
// Create a builder for your application component:
//
//	builder := bedrock.BuilderFunc[MyApp](func(ctx context.Context) (MyApp, error) {
//	    return MyApp{}, nil
//	})
//
// Create a runtime by transforming the builder:
//
//	runtime := bedrock.Map(builder, func(app MyApp) (bedrock.Runtime, error) {
//	    return bedrock.RuntimeFunc(func(ctx context.Context) error {
//	        return app.Start(ctx)
//	    }), nil
//	})
//
// Run the application with signal handling and panic recovery:
//
//	runner := bedrock.RecoverPanics(
//	    bedrock.NotifyOnSignal(
//	        bedrock.DefaultRunner[bedrock.Runtime](),
//	        os.Interrupt,
//	    ),
//	)
//	if err := runner.Run(context.Background(), runtime); err != nil {
//	    log.Fatal(err)
//	}
package bedrock
