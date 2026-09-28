// This is a comment
// GO ROUTINES

package main

import (
	"fmt" // formatting output
	"time" // import time
)

// define a function
//	take in var named from of type string
//	:= (equals)
func f(from string) {
	for i := 0; i < 6; i++ {
		fmt.Println(from, ":", i)
	}
}

// main function calling f with a string input
func main() {
	f("direct")
	go f("goroutine")
	time.Sleep(2*time.Second)
	
}
