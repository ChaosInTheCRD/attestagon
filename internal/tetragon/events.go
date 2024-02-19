package tetragon

type TetragonEvent struct {
	PodName      string
	PodNamespace string
	Type         string
	Body         interface{}
}
