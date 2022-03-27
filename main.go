package main

import (
	"fmt"
	"github.com/bxsec/gotool/util"
)

func main() {
	uid := util.GetUID()
	fmt.Println("uid:", uid)
}

