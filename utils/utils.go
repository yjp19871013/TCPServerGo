package utils

import "strings"

func IsStringEmpty(str string) bool {
	return strings.Trim(str, " ") == ""
}
