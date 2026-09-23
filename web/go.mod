// This file only stops the Go tooling from descending into web/ (node_modules ships stray Go
// packages). web/ is TypeScript; it is not a Go module and never will be.
module orpheum.invalid/web-is-not-go
