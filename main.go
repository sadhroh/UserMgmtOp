package main

import (
  "os"
  "flag"
  "fmt"
  "UserMgmtOp/usermgmt"
)

func main() {

  // Check for minimum required arguments
  if len (os.Args) == 1{
    fmt.Println ("Too few argumnts. Use -h or --help to see available options")
    os.Exit(1)
  }

  // Register the flags for executing the binary
  addFlagPtr := flag.Bool("a", false, "Add User")
  delFlagPtr := flag.Bool("d", false, "Delete User")
  updtFlagPtr := flag.Bool("u", false, "Update User")
  passFlagPtr := flag.String("pass", "", "Password")
  userFlagPtr := flag.String("user", "", "Username")
  newPassFlagPtr := flag.String("npass", "", "New Password. Use with -u option")
  newUserFlagPtr := flag.String("nuser", "", "New Username. Use with -u option")

  flag.Parse()

  // Check for existing username and password to be provided with flags
  if *userFlagPtr == "" || *passFlagPtr == "" {
    fmt.Println("Missing required details")
    os.Exit (1)
  }

  // Create necesssary channels to communicate accross goroutines
  userNameChan := make (chan string)
  userPassChan := make (chan string)
  opStatusChan := make (chan bool)

  defer close (userNameChan)
  defer close (userPassChan)
  defer close (opStatusChan)


  // Create an instance of the user management type.
  // This is used to trigger subsequent goroutines and perform gievn operations.
  var userMgmt usermgmt.UserCred

  // Trigger the validation service as goroutine
  go userMgmt.ValidateUser(userNameChan, userPassChan, opStatusChan)

  // Pass the username & password in the channel to be used by the goroutines
  userNameChan<- *userFlagPtr
  userPassChan<- *passFlagPtr

  // Check which operation was specified via the flag
  // Executing the below switch in a goroutine may lead to sudden termination
  // when updating an existing record due to receive from channel to signal update
  // continue or exit due to username already used.
  switch {
  case *updtFlagPtr:
    // For update operation, additional username & password
    // is required to be updated by replacing the old ones.
    // The new ones can be the same as the current user
    // credentials. If the new username is already present,
    // the operation is stopped and a message logged.
    if *newUserFlagPtr == "" || *newPassFlagPtr == "" {
      fmt.Println("Missing new arguments for update")
      os.Exit (1)
    }
    go userMgmt.UpdateUser ()
    if !<-opStatusChan{
      fmt.Println ("Unable to update")
      return
    }
    // Send the new credentials in the channels
    userNameChan<- *newUserFlagPtr
    userPassChan<- *newPassFlagPtr
  case *delFlagPtr:
    go userMgmt.DeleteUser ()
  case *addFlagPtr:
    go userMgmt.AddUser ()
  default:
    fmt.Println ("Invalid operation specified")
    os.Exit(1)
  }

  // Wait for the entire operation leg to complete and display
  // appropriate response indicating overall success or failure.
  if <-opStatusChan{
    fmt.Println ("Operation completed")
  } else {
    fmt.Println ("Operation failure")
  }
}
