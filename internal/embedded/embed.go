package embedded

import _ "embed"

//go:embed tsk.default.yml
var DefaultConfig []byte
