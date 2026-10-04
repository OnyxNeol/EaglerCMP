//go:build bundled

package bundle

import _ "embed"

//go:embed client.tar.gz
var archive []byte

//go:embed source.txt
var source string
