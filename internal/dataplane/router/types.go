package router

type Decision uint8
type MethodMask uint64

type CompileRoute struct {
	MethodMask MethodMask
}
