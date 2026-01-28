package xjson_test

import (
	"fmt"

	"github.com/omeyang/xkit/pkg/util/xjson"
)

func ExamplePretty() {
	type User struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	fmt.Println(xjson.Pretty(User{Name: "Alice", Age: 30}))
	// Output:
	// {
	//   "name": "Alice",
	//   "age": 30
	// }
}
