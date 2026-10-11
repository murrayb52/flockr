// Package integrations ships the agent-side integration files inside the binary.
package integrations

import _ "embed"

//go:embed pi/flock.ts
var PiExtension []byte
