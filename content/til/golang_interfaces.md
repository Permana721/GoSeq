---
Title: Golang Interfaces — A TIL
Date: 2026-10-27
Lang: EN
Category: Go
---
- Interfaces in Go are satisfied *implicitly* — no `implements` keyword needed
  - Any type that has all the methods of an interface automatically satisfies it
  - This enables duck typing and loose coupling between packages
- Common patterns
  - Define small, focused interfaces (`io.Reader`, `io.Writer`)
  - Accept interfaces, return concrete types
  - Use `interface{}` (or `any`) sparingly — prefer typed interfaces
- Example
  ```go
  type Stringer interface {
      String() string
  }

  type Person struct {
      Name string
  }

  func (p Person) String() string {
      return p.Name
  }
  ```
- The zero value of an interface is `nil` — always check before calling methods
- [[Go]] [[Interfaces]] #go #programming
