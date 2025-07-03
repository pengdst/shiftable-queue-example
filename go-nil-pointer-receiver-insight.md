# Go Nil Pointer Receiver: What I Learned

I recently learned something important about how Go handles method calls on nil pointer receivers, and it's a common source of confusion. Here's my personal summary, in plain English, with examples.

## 1. Calling Methods on Nil Pointer Receivers Does NOT Panic (Unless You Dereference)

In Go, I can call a method with a pointer receiver on a variable that is nil, and it will NOT panic as long as the method does not access (dereference) any data from the receiver.

**Example:**
```go
type S struct{}
func (s *S) Hello() string {
    return "hello"
}

var s *S = nil
fmt.Println(s.Hello()) // Output: hello, does NOT panic
```
This is not a new feature; it's been Go's behavior from the start.

## 2. It WILL Panic If the Method Dereferences the Receiver

If the method tries to access a field or another method of the receiver (e.g., `s.X`, `s.Y()`), Go needs to dereference the pointer. If the pointer is nil, this will panic with `invalid memory address or nil pointer dereference`.

**Example:**
```go
type S struct{ X int }
func (s *S) Value() int {
    return s.X // this dereferences the receiver
}

var s *S = nil
fmt.Println(s.Value()) // PANICS
```

## 3. Defensive Coding in Libraries (e.g., zerolog)

Many Go libraries, like zerolog, add guard clauses at the start of pointer receiver methods:

```go
func (e *Event) Msgf(format string, v ...interface{}) {
    if e == nil {
        return
    }
    e.msg(fmt.Sprintf(format, v...))
}
```
So, even if I call `.Msgf()` on a nil pointer, it just returns immediately—no dereference, no panic. This is not Go runtime magic, but intentional defensive coding by the library author.

## 4. My Takeaways

- In Go, calling a method on a nil pointer receiver is safe as long as the method does not dereference the receiver.
- If the method accesses a field or another method of the receiver, it will panic.
- Many libraries add guard clauses to make pointer receiver methods always safe to call, even on nil pointers.
- I should never assume that calling a pointer receiver method on nil will always panic. Always check the method's implementation or the library's source code.

**Quick Recap:**
```go
type S struct{ X int }
func (s *S) Safe() string { return "OK" }
func (s *S) Unsafe() int  { return s.X }

var s *S = nil
_ = s.Safe()   // does NOT panic
_ = s.Unsafe() // PANICS
```

**Bottom line:**
Whether a method call on a nil pointer receiver panics in Go depends on what the method does—not just on the pointer being nil. Many Go libraries add extra guardrails to make their APIs safer. Always check the code before making assumptions. 