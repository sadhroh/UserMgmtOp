package usermgmt

import (
  "fmt"
  "log"
  "UserMgmtOp/fileop"
  "strings"
  "crypto/sha256"
)

// UserCred is the type that enables the connection between main
// program with the user management methods, to be used for
// required operations like Validate, Add, Update & Delete.
type UserCred struct{
  Username, Password string
}

// Common variables to the functions
// credMap --> map of usernames vs passwords obtained from parsing the file
// opSPecChan --> channel to send the opearation specification to the file handler to
//                get the change relfected in the record file
// valSuccessChan --> bi-directional channel used to signal the package level functions,
//                    triggered as goroutines, about the validation status
// signalControllerChan --> send to channel of type boolean to signal the main
//                          program about the operation status
// userNameChanIn & userPassChanIn --> channels connected to the main program to receive
//                                     the usernam & password respectively
var (
  credMap = map[string]string{}
  opSPecChan = make (chan fileop.OpSpec)
  valSuccessChan = make (chan bool)
  signalControllerChan chan<- bool
  userNameChanIn, userPassChanIn <-chan string
)

// init function initializes package level global variables
// with necessary values as well as prepares the methods to
// perform operation with the record data.
func init (){
  go fileop.ParseRecordToMap(&credMap, valSuccessChan)
}

// ValidateUser authenticates the user by checking the username and password
// given as input via the channels, by looking for the matching record in the
// record data file.
// It connects appropriate channels to interface between the main & package level
// programs so that they may operate via goroutines.
func (uscred *UserCred) ValidateUser (userNameChan, userPassChan <-chan string, valid chan<- bool){

  // Wait till file parsing is in progress
  <-valSuccessChan

  // Obtain the username & password from the channels
  uscred.Username = <-userNameChan
  uscred.Password = <-userPassChan

  // Connect the channels from the main to local channels for direct communication
  userNameChanIn = userNameChan
  userPassChanIn = userPassChan
  signalControllerChan = valid

  // Generate the SHA256 hash of the password to check against the record data
  h := sha256.New()
  h.Write([]byte(uscred.Password))
  uscred.Password = fmt.Sprintf ("%x", h.Sum (nil))

  if passFrmMap, ok := credMap[uscred.Username]; ok{
    if strings.Compare (passFrmMap, uscred.Password) == 0{
      // Corresponding record obtained
      log.Println ("Access granted for", uscred.Username)
      // Signal waiting goroutines to proceed with the operations
      valSuccessChan<- true
    } else {
      // Password for the username is incorrect.
      log.Println ("Wrong password for", uscred.Username)
    }
  } else {
    // Username does not match any record.
    log.Println ("Access denied for", uscred.Username)
  }
  valSuccessChan<- false
}

// AddUser adds a new user to the record if it is not already present.
// If present, it does not do anything but logs a message.
func (uscred *UserCred) AddUser (){
  log.Println ("Attempting to add new user")

  // Wait for the validation function to complete
  if <-valSuccessChan || credMap[uscred.Username] != ""{
    log.Println ("User already present. Not added...")
    signalControllerChan<- true
    return
  } else {
    credMap[uscred.Username] = uscred.Password
    tempUsCred := fileop.UserCred{Username: uscred.Username,
                                  Password: uscred.Password}
    go fileop.DumpDataToFile (opSPecChan, signalControllerChan)
    opSpec := fileop.OpSpec{ OpType: fileop.USR_ADD,
                       NewData: &tempUsCred, }
    opSPecChan<- opSpec
  }
}

// DeleteUser removes an existing user from the records
// if present else quits and signals operation failure.
func (uscred *UserCred) DeleteUser (){
  log.Println ("Attempting to delete user")

  // Wait for the validation function to complete
  if !<-valSuccessChan {
    signalControllerChan<- false
    return
  }
  go fileop.DumpDataToFile (opSPecChan, signalControllerChan)
  tempUsCred := fileop.UserCred{Username: uscred.Username,
                                Password: uscred.Password}
  opSpec := fileop.OpSpec{ OpType: fileop.USR_DEL,
                     OldData: &tempUsCred, }
  delete (credMap, uscred.Username)
  opSPecChan<- opSpec
}

// UpdateUser updates the current username & password with the new ones.
// The operation is only done when the new username is not already taken
// by a different user. The current record is removed and a fresh record
// with the newly supplied username and password is captured.
// The new username & password is obtained from the channels directly from
// main program.
func (uscred *UserCred) UpdateUser (){
  log.Println ("Attempting to update user")

  // Wait for the validation function to complete
  if !<-valSuccessChan {
    signalControllerChan<- false
    return
  }
  signalControllerChan<- true

  // Wait for the new username & password from main
  newUserName := <-userNameChanIn
  newPassword := <-userPassChanIn

  // Check if the new username is present & not the current username.
  if _, ok := credMap[newUserName]; ok && strings.Compare(newUserName, uscred.Username) != 0{
    log.Printf ("Username <%s> is already taken", newUserName)
    signalControllerChan<- false
    return
  }

  go fileop.DumpDataToFile (opSPecChan, signalControllerChan)

  tempUsCred := fileop.UserCred{Username: uscred.Username,
                                Password: uscred.Password}
  opSpec := fileop.OpSpec{ OpType: fileop.USR_UPD,
                     OldData: &tempUsCred, }

  // Remove the present record
  delete (credMap, uscred.Username)

  // Create a SHA256 hash of the new password
  h := sha256.New()
  h.Write([]byte(newPassword))
  uscred.Password = fmt.Sprintf ("%x", h.Sum (nil))
  uscred.Username = newUserName

  // Put it in the record
  credMap[uscred.Username] = uscred.Password
  tempUsCredNew := fileop.UserCred{Username: uscred.Username,
                                Password: uscred.Password}
  opSpec.NewData = &tempUsCredNew
  opSPecChan<- opSpec
}
