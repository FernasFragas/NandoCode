package sample

type Service struct{}

func (s *Service) Login(user string) bool {
	return user != ""
}

func helper() int {
	return 42
}
