package main

import "fmt"

type People struct {
}

func (p People) Print(str string) {
	fmt.Println("people")
}

type Student struct {
	People
}

func (stu *Student) Print(str string) {
	if str == "boy" {
		fmt.Println("a boy")
	} else {
		fmt.Println("a people")
	}
}

func main() {
	p := Student{}
	p.Print("boy")
}
