package output

import(
	"encoding/json"
	"fmt"
	
)

func PrintJsonOutput(output interface{}) {
	bs, err := json.MarshalIndent(output, "", "    ")
	if err == nil {
		fmt.Println(string(bs))
	}
}