package addinputvalidation

import (
	"fmt"
	"strings"
)

type User struct {
	Name string
}

func BuildUser(name string) (User, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return User{}, fmt.Errorf("name is required")
	}
	return User{Name: name}, nil
}
