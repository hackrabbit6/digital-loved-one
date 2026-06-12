//go:build objectbox

package memory

//go:generate go run github.com/objectbox/objectbox-go/cmd/objectbox-gogen

// ObjectBoxPersona stores persona profile JSON as an ObjectBox entity.
type ObjectBoxPersona struct {
	ID   uint64 `objectbox:"id"`
	Key  string `objectbox:"unique"`
	Data string
}

// ObjectBoxExcerpt stores source excerpt JSON as an ObjectBox entity.
type ObjectBoxExcerpt struct {
	ID        uint64 `objectbox:"id"`
	Key       string `objectbox:"unique"`
	PersonaID string `objectbox:"index"`
	Data      string
}

// ObjectBoxTopic stores topic node JSON as an ObjectBox entity.
type ObjectBoxTopic struct {
	ID        uint64 `objectbox:"id"`
	Key       string `objectbox:"unique"`
	PersonaID string `objectbox:"index"`
	Label     string `objectbox:"index"`
	Data      string
}
