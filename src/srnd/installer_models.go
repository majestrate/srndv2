package srnd

type ErrorModel struct {
	Err error
}

func (self *ErrorModel) Error() string {
	return self.Err.Error()
}

func (self *ErrorModel) HasError() bool {
	return self.Err != nil
}

type StepModel struct {
	Node *dialogNode
}

func (self *StepModel) HasNext() bool {
	return len(self.Node.children) > 0
}

func (self *StepModel) HasPrevious() bool {
	return self.Node.parent != nil
}

type BaseDialogModel struct {
	ErrorModel
	StepModel
}

type DBModel struct {
	ErrorModel
	StepModel

	username string
	host     string
	port     string
}

func (self *DBModel) Username() string {
	return self.username
}

func (self *DBModel) Host() string {
	return self.host
}

func (self *DBModel) Port() string {
	return self.port
}
