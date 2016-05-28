package model

type Article struct {
	Subject     string
	Name        string
	Header      map[string][]string
	Text        string
	Attachments []Attachment
}
