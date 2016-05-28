package frontend

// ( message-id, references, newsgroup )
type Post [3]string

func (p Post) MessageID() string {
	return p[0]
}

func (p Post) Reference() string {
	return p[1]
}

func (p Post) Newsgroup() string {
	return p[2]
}
