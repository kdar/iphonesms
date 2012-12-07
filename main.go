package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"
	//"reflect"
	"github.com/kdar/iphonesms/utility"
	"io/ioutil"
	"path/filepath"
	"time"
	//. "github.com/sdegutis/sqlite_go_wrapper"
	//"guthub.com/kuroneko/sqlite3"
	sqlite "github.com/gwenn/gosqlite"
)

var NONDIGIT_REGEXP = regexp.MustCompile("[^0-9]+")

const (
	BACKUP_PATH = "$APPDATA\\Apple Computer\\MobileSync\\Backup"
	SMS_FILE    = "3d0d7e5fb2ce288813306e4d4636395e047a3d28" //Text Messages
	AB_FILE     = "31bb7ba8914766d4ba40d6dfb6113c8b614be442" //Contacts
	OUTPUT_DIR  = "output"
)

const (
	MESSAGE_RECEIVED = 0x2
	MESSAGE_SENT     = 0x3
)

type Message struct {
	ROWID          int
	Address        string
	Date           int64
	Text           string
	Flags          int
	Replace        int
	Svc_center     string
	Group_id       int
	Association_id int
	Height         int
	UIFlags        int
	Version        int
	Subject        string
	Country        string
	Headers        string
	Recipients     string
	Read           int

	// New Iphone iOS 5
	Smsc_ref              int
	Dr_date               int
	Madrid_attributedBody string
	Madrid_handle         string
	Madrid_version        int
	Madrid_guid           string
	Madrid_type           int
	Madrid_roomname       string
	Madrid_service        string
	Madrid_account        string
	Madrid_flags          int
	Madrid_attachmentInfo string
	Madrid_url            string
	Madrid_error          int
	Is_madrid             bool
	Madrid_date_read      int64
	Madrid_date_delivered int64
	Madrid_account_guid   string

	// Extra
	Contact    Contact
	Outgoing   bool
	TimeStruct time.Time
	TimeDate   string
	Time       string
}

type Contact struct {
	Address  string
	First    string
	Last     string
	Middle   string
	Nickname string

	Identifier string
}

type Messages struct {
	Data []Message
}

type IEmpty interface{}

//===================================
func (c *Contact) Name() string {
	var name string
	if len(c.First) != 0 {
		name += c.First
	}

	if len(c.Middle) != 0 {
		name += " " + c.Middle
	}

	if len(c.Last) != 0 {
		name += " " + c.Last
	}

	return strings.Trim(name, " ")
}

//===================================
func (c *Contact) String() string {
	return format_address(c.Address)
}

//===================================
func (c *Contact) Process() {
	//c.Address = c.Address

	c.Identifier = strings.Trim(c.Name(), " ")
	if len(c.Identifier) == 0 {
		c.Identifier = c.String()
	}
}

//===================================
func (m *Message) Process() {
	//incoming_flags := []int{12289, 77825}
	outgoing_flags := []int{36869, 102405}

	if m.Is_madrid {
		m.Address = m.Madrid_handle
		if m.Madrid_date_delivered != 0 {
			m.Date = fix_imessage_date(m.Madrid_date_delivered)
		} else {
			m.Date = fix_imessage_date(m.Madrid_date_read)
		}
	}

	if m.Is_madrid {
		if utility.Contains(m.Madrid_flags, outgoing_flags) {
			m.Outgoing = true
		}
	} else {
		if m.Flags&MESSAGE_SENT == MESSAGE_SENT {
			m.Outgoing = true
		}
	}

	m.TimeStruct = time.Unix(m.Date, 0) //*time.SecondsToUTC(m.Date)
	m.TimeDate = m.TimeStruct.Format("2006-01-02")
	m.Time = m.TimeStruct.Format("15:04:05")
}

//===================================
func fix_imessage_date(seconds int64) int64 {
	/*
	   Convert seconds to unix epoch time.

	   iMessage dates are not standard unix time.  They begin at midnight on
	   2001-01-01, instead of the usual 1970-01-01.

	   To convert to unix time, add 978,307,200 seconds!

	   Source: http://d.hatena.ne.jp/sak_65536/20111017/1318829688
	   (Thanks, Google Translate!)
	*/
	return seconds + 978307200
}

//===================================
func format_address(address string) string {
	if !strings.Contains(address, "@") && !strings.Contains(address, ":") {
		address = NONDIGIT_REGEXP.ReplaceAllString(address, "")
		if len(address) > 0 && address[0] == '1' {
			address = address[1:]
		}
	}

	return strings.Trim(address, " ")
}

//===================================
func main() {
	sep := "\\" //filepath.Separator
	fi, err := ioutil.ReadDir(os.ExpandEnv(BACKUP_PATH))

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if len(fi) == 0 {
		fmt.Println("Please sync your iPhone using iTunes first.")
		os.Exit(1)
	}

	m_path := os.ExpandEnv(BACKUP_PATH + string(sep) + fi[0].Name() + string(sep) + SMS_FILE)
	m_conn, err := sqlite.Open(m_path)
	if err != nil {
		fmt.Println("Unable to open the Message database: %s", err)
		os.Exit(1)
	}
	defer m_conn.Close()

	ab_path := os.ExpandEnv(BACKUP_PATH + string(sep) + fi[0].Name() + string(sep) + AB_FILE)
	ab_conn, err := sqlite.Open(ab_path)
	if err != nil {
		fmt.Println("Unable to open the AddressBook database: %s", err)
		os.Exit(1)
	}
	defer ab_conn.Close()

	// Gather all of the contacts up front.
	cmap := make(utility.SMap)
	ab_stmt, err := ab_conn.Prepare("select First,Middle,Last,Nickname,value from ABPerson p LEFT JOIN ABMultiValue a ON p.ROWID = a.record_id;")
	defer ab_stmt.Finalize()
	//ab_stmt.Exec()
	for {
		more, _ := ab_stmt.Next()
		if more {
			contact := &Contact{}
			ab_stmt.Scan(&contact.First, &contact.Middle, &contact.Last, &contact.Nickname, &contact.Address)
			if err != nil {
				fmt.Printf("Error while getting AddressBook row data: %s\n", err)
				os.Exit(1)
			}

			contact.Process()
			cmap.Insert(contact, contact)
		} else {
			break
		}
	}

	//v := reflect.TypeOf(Message{})
	//fmt.Printf("%s, %s\n", v.NumField(), v.Field(0).Name)

	m_stmt, err := m_conn.Prepare("select address,text,flags,date,is_madrid,madrid_flags,madrid_handle,madrid_date_delivered,madrid_date_read from message ORDER BY rowid")
	defer m_stmt.Finalize()
	//err = m_stmt.Exec()
	// if err != nil {
	//   fmt.Println("Error while selecting: %s", err)
	// }

	tmpl, _ := template.ParseFiles("output.tmpl")

	var messages Messages
	var lastMessage Message
	var currentPath string
	var f *os.File
	for {
		more, _ := m_stmt.Next()
		var message Message

		if more {
			m_stmt.Scan(
				&message.Address,
				&message.Text,
				&message.Flags,
				&message.Date,
				&message.Is_madrid,
				&message.Madrid_flags,
				&message.Madrid_handle,
				&message.Madrid_date_delivered,
				&message.Madrid_date_read)

			if err != nil {
				fmt.Printf("Error while getting Message row data: %s\n", err)
				//os.Exit(1)
			}

			if len(message.Address) == 0 && len(message.Madrid_handle) == 0 {
				continue
			}

			message.Process()

			faddress := format_address(message.Address)
			contact, ok := cmap[faddress]
			if ok {
				//fmt.Println(contact.Value.(*Contact))
				message.Contact = *contact.Value.(*Contact)
			} else {
				message.Contact = *&Contact{Identifier: faddress}
			}
		}

		if !more || message.Address != lastMessage.Address || (len(lastMessage.TimeDate) != 0 && message.TimeDate != lastMessage.TimeDate) {
			currentPath = filepath.Join(OUTPUT_DIR, message.Contact.Identifier)
			os.MkdirAll(currentPath, 0666)

			if f != nil {
				tmpl.Execute(f, messages)
				f.Close()
			}

			if more {
				f, err = os.OpenFile(filepath.Join(currentPath, message.TimeDate+".txt"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
				messages.Data = nil
			}
		}

		messages.Data = append(messages.Data, message)
		lastMessage = message

		if !more {
			break
		}

		//		results, err := m_stmt.ResultsAsMap()
		//    if err == nil {
		//		  fmt.Printf("%v\n", string(results["text"]))
		//		}
	}

	defer f.Close()
}
