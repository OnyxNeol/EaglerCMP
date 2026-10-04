// Package bundle optionally carries an Eaglercraft web client inside the
// launcher binary, so release builds install and run with no network access.
// CI fetches the client from the upstream repo into bundle/client.tar.gz and
// builds with -tags=bundled; ordinary builds embed nothing.
package bundle

import "strings"

// Archive returns the embedded client as a .tar.gz, or nil if the binary was
// built without the "bundled" tag.
func Archive() []byte { return archive }

// Source describes where the embedded client was fetched from.
func Source() string { return strings.TrimSpace(source) }
