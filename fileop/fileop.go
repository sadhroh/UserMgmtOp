package fileop

import (
  "log"
  "os"
  "bufio"
  "strings"
  "sort"
  "sync"
  "fmt"
)

// RECFILE holds the path to the record file
// The file must be in csv format(comma-separated-values).
const (
  RECFILE = "/Misc/recFile.csv"
)

// unameSlc stores the sorted list of usernames obtained while creating the map
var(
  unameSlc []string
)

// UserCred is the type that enables the connection between main
// program with the user management methods, to be used for
// required operations like Validate, Add, Update & Delete.
type UserCred struct{
  Username, Password string
}

// The constants denote the type of operation done.
// This is mainly to signal the file handler process what has been
// done so that the same can be reflected in the record data file.
const (
  USR_ADD = iota
  USR_UPD
  USR_DEL
)

// OpSpec specifies the atomic operation done on the data.
// This is meant to be sent to the file handler process to
// supply a instruction and state of the data before & after
// the required operation is performed.
type OpSpec struct{
  OpType int
  OldData *UserCred
  NewData *UserCred
}

// getRecFile opens the file containing the user records in the specified mode
// The path to the file is given as per global constant variable, RECFILE.
// If the file is not not present, the function creates the file in the go
// workspace as mentioned by the GOPATH environment variable.
func getRecFile (mode int) (f *os.File){

  fileName := strings.Split(os.Getenv ("GOPATH"), ":")[0] + "/" + RECFILE

  if _, err := os.Stat(fileName); os.IsNotExist(err){

    // Create the directory path if not present
    if strings.Count(fileName, "/") >= 0{
      dirPath := fileName[:strings.LastIndex (fileName, "/")]
      if os.MkdirAll(dirPath, 0644) != nil{
        log.Fatal ("Could not create path to record file")
      }
    }

    f, err = os.Create(fileName)
    if err != nil{
      log.Fatal (err)
    }
    f.Close()
    log.Println ("Path to file:", fileName)
  }

  f, err := os.OpenFile (fileName, mode, 0644)
  if err != nil{
    log.Fatal (err)
  }
  return
}

// DumpDataToFile is meant to be executed as a go routine using one receive from
// and one send to channel. The receive from channel specifies the atomic
// operation to be done on the records in the file. It expects a structure of
// type OpSpec containing the details of the operation.
// The send to channel is to communicate the completion of the task.
// The main task of this function is to update the record file with the changed
// map values. It takes care of synchronizing the records, in case any operations
// were performed on the file.
// 2 goroutines are triggered, one to dump the formatted data to the file &
// another to send sync & send the data to be dumped.
func DumpDataToFile(opSPecChan <-chan OpSpec, done chan<- bool){

  var wg sync.WaitGroup

  recChan := make (chan string)
  tempChan := make (chan bool)

  credMap := map[string]string{}

  // Get the fresh list of records from the file, if modified.
  go ParseRecordToMap (&credMap, tempChan)

  // Get the operation specification
  opSPec := <-opSPecChan

  // Wait for the fresh list of data to be populated
  <-tempChan

  wg.Add(2)

  // Goroutine to dump data to the file.
  go func (recChan <-chan string){
    defer wg.Done()
    fp := getRecFile(os.O_WRONLY|os.O_TRUNC)
    if fp == nil{
      log.Fatal("Unable to obtain file pointer")
    }
    defer fp.Close()
    writer := bufio.NewWriter(fp)

    // Continue writing untill the channel is closed
    for rec := range recChan{
      writer.WriteString (rec)
    }

    // Always better to explicitly flush the data to finish writing
    writer.Flush()
  }(recChan)

  // Goroutine to sync the current process data with the fresh data.
  // Can be made pretty by creating a separate function[check later]
  go func (recChan chan<- string){
    defer wg.Done()
    dataChanged := false

    // Loop through the freshly obtained sorted list of usernames
    for _, uname := range unameSlc{

      // Check the operation type
      switch (opSPec.OpType){

      // ADD
      case USR_ADD:

        // When a new record is added, check if the new username is lexically lesser
        // than the existing username in the file, and only then send the record.
        if strings.Compare(opSPec.NewData.Username, uname) < 0 && !dataChanged{
          recChan<- fmt.Sprintf ("%s,%s\n", opSPec.NewData.Username, opSPec.NewData.Password)
          dataChanged = true
        }

      // UPDATE
      case USR_UPD:

        // When update operation is done, keep a check if the newusername(may not be new),
        // is lexically lesser(in case of new) or equal(if same), and send the record
        // to the writing goroutine to be dumped.
        if strings.Compare(opSPec.NewData.Username, uname) <= 0 && !dataChanged{
          recChan<- fmt.Sprintf ("%s,%s\n", opSPec.NewData.Username, opSPec.NewData.Password)
          dataChanged = true
        }

        // If the old record is obtained from the fresh list, skip to the next.
        if strings.Compare(opSPec.OldData.Username, uname) == 0{
          continue
        }

      // DELETE
      case USR_DEL:

        // When a record is deleted, skip sending the data to be dumped
        if strings.Compare(opSPec.OldData.Username, uname) == 0 && !dataChanged{
          dataChanged = true
          continue
        }
      }
      recChan<- fmt.Sprintf ("%s,%s\n", uname, credMap[uname])
    }

    // When the record is to be inserted at the end
    if !dataChanged && opSPec.NewData != nil{
      recChan<- fmt.Sprintf ("%s,%s\n", opSPec.NewData.Username, opSPec.NewData.Password)
    }
    close (recChan)
  }(recChan)
  wg.Wait()

  done<- true
}

// ParseRecordToMap reads the records from the file and dumps the entire
// data to a map by parsing the username & password fields. It also maintains
// sorted list of usernames as an unexported slice in the global space.
// Though meant to be executed as a go routine, many times, synchronization
// is important than concurrency, specially when waiting for the data dump
// to be done. Thus, the channel is provided and only operates when an actual
// channel is established from the caller.
func ParseRecordToMap (credMap *map[string]string, done chan<- bool){
  // Obtain the file pointer to read the file
  fp := getRecFile(os.O_RDONLY)
  if fp == nil{
    log.Fatal("Unable to obtain file pointer")
  }
  defer fp.Close()

  // Every time the function is called, fresh data from the file
  // is taken and the usernames are cached as a slice.
  unameSlc = nil

  sc := bufio.NewScanner(fp)
  for sc.Scan(){
    strSlc := strings.Split(sc.Text(), ",")

    // 2 comma-separated fields are necessary.
    // Other cases denote file corruption & integrity compromise.
    if len(strSlc) != 2{
      log.Fatalf("Record file corrupted")
    }
    (*credMap)[strSlc[0]] = strSlc[1]
    unameSlc = append (unameSlc, strSlc[0])
  }

  // Sort the slice of usernames.
  if !sort.StringsAreSorted(unameSlc){
    sort.Strings(unameSlc)
  }

  if sc.Err() != nil{
    log.Fatal(sc.Err())
  }

  // Only send completion signal if actual channel was
  // established from the caller end, else ignore.
  if done != nil{
    done<- true
  }
}
