package addinputvalidation

type User struct {
	Name string
}

func BuildUser(name string) User {
	return User{Name: name}
}
