package shapeset

import (
	"fmt"
	"github.com/fatih/color"
	"os"
	"strconv"
)

func assert(statement string, validity interface{}) {
	if debug_level() < 1 {
		return
	}
	var not_valid bool
	if lambda, ok := validity.(func() bool); ok {
		not_valid = !lambda()
	} else if boolean, ok := validity.(bool); ok {
		not_valid = !boolean
	}
	if not_valid {
		fmt.Print("\a") // bell
		red := color.New(color.FgRed).SprintFunc()
		panic(red("Assertion failed: " + statement))
	}
}

func debug_level() (debug_level int64) {
	debug_level, _ = strconv.ParseInt(os.Getenv("DEBUG_LEVEL"), 10, 64)
	return
}
