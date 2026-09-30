// Command hub-clone-tool clones all repositories a user has access to from GitHub.
package main

import (
	"os"

	"github.com/paul-ridgway/hub-clone-tool/internal/hct"
)

func main() {
	os.Exit(hct.Run(os.Args))
}
