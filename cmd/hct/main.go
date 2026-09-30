// Command hct is a short alias for hub-clone-tool.
package main

import (
	"os"

	"github.com/paul-ridgway/hub-clone-tool/internal/hct"
)

func main() {
	os.Exit(hct.Run(os.Args))
}
