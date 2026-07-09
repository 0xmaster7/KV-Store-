package main

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

var BUCKET_SIZE = 100
var ENTRIES = 0
var WAL_ENTRIES = 0
var SSTable_index = 1

// type <name of struct> struct
type Entry struct {
	key string
	val string
}

type IndexEntry struct {
	key    string
	offset int64
}

var table = make([][]Entry, BUCKET_SIZE)

func hash(key string) uint64 {
	hash_obj := fnv.New64a()
	hash_obj.Write([]byte(key))
	return hash_obj.Sum64()
}

func get_sstable_index() {
	files, err := os.ReadDir(".")
	if err != nil {
		fmt.Println("error loading dir files")
	}
	mx := 0
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "sstable_") && strings.HasSuffix(file.Name(), ".log") {
			res := strings.TrimPrefix(file.Name(), "sstable_")
			res = strings.TrimSuffix(res, ".log")
			num, _ := strconv.Atoi(res)
			mx = max(num, mx)
		}
	}
	SSTable_index = mx + 1

}
func PUT(key1 string, value string) {
	hashed_key := hash(key1)
	bucket := int(hashed_key % uint64(BUCKET_SIZE))
	found := 1
	for index, entry := range table[bucket] {
		if entry.key == key1 {
			table[bucket][index].val = value //table -> bucket->inside bucket multiple entries->index of entry
			found = 0
			break
		}
	}
	if found == 0 {
		fmt.Println("successfully updated key present in bucket !")
	} else if found == 1 {
		table[bucket] = append(table[bucket], Entry{key: key1, val: value})
		ENTRIES++
		fmt.Println("successfully added key-value to bucket!")
		resize()
	}
}

// binary search cus keys r sorted in sstables
// now using sparse indexing so no more O(n) for searching through sstables MUCH more efficient
func GET(key1 string) {
	hashed_key := hash(key1)
	bucket := int(hashed_key % uint64(BUCKET_SIZE))
	for index, entry := range table[bucket] {
		if entry.key == key1 {
			fmt.Printf("Value is = %s\n", table[bucket][index].val)
			return
		}
	}

	for i := SSTable_index - 1; i >= 1; i-- {
		file_name := fmt.Sprintf("sstable_%d.index", i)

		sstable_file, err := os.Open(file_name)
		if err != nil {
			fmt.Println("Error opening sstable_file")
			continue //cus if error then it skips this ssable file and goes to the next sstable file return gets out of func immediately
		}
		scanner := bufio.NewScanner(sstable_file)
		list := make([]IndexEntry, 0)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.Fields(line)

			if len(parts) == 2 {
				num, _ := strconv.ParseInt(parts[1], 10, 64) //parts[1] is str in base 10 as 64 bit int
				list = append(list, IndexEntry{
					key:    parts[0],
					offset: num,
				})
			}
		}
		sstable_file.Close()

		left := 0
		right := len(list) - 1
		best := -1 //variable to keep track of closest offse

		for left <= right {
			mid := (left + right) / 2

			if list[mid].key <= key1 {
				best = mid
				left = mid + 1

			} else {
				right = mid - 1
			}
		}

		if best == -1 {
			continue
		}
		offset := list[best].offset
		file_name2 := fmt.Sprintf("sstable_%d.log", i)

		sstable_log_file, err := os.Open(file_name2)

		if err != nil {
			fmt.Println("Error opening sstable log file")
			continue
		}
		_, err = sstable_log_file.Seek(offset, io.SeekStart)

		if err != nil {
			fmt.Println("Error seeking to offset")
			continue
		}

		scanner1 := bufio.NewScanner(sstable_log_file)
		for scanner1.Scan() {
			line := scanner1.Text()
			parts := strings.Fields(line)

			if len(parts) == 3 {
				if parts[1] == key1 {
					fmt.Printf("Value is %s\n", parts[2])
					sstable_log_file.Close()
					return
				} else if parts[1] > key1 {
					//parts[1] cannot be > key1 cus the log file is sorted
					break
					//key dosent exist so break
				}
			}
		}
		sstable_log_file.Close()

	}
	fmt.Println("key not found")

}

func DELETE(key1 string) {
	hashed_key := hash(key1)
	bucket := int(hashed_key % uint64(BUCKET_SIZE))
	found := 1
	for index, entry := range table[bucket] {
		if entry.key == key1 {
			table[bucket] = append(table[bucket][:index], table[bucket][index+1:]...)
			found = 0
			ENTRIES--
			//fmt.Printf("Value is = %s\n", table[bucket][index].val) cant do thisi cus after line 66 index may point to a diff thing
			fmt.Printf("Value is = %s\n", entry.val)
			break
		}
	}
	if found == 0 {
		fmt.Println("Element successfully deleted!")
	} else {
		fmt.Println("Element not found")
	}
}

func resize() {
	//every entry in a bucket has to be rehashed and appended to new table
	if float64(ENTRIES)/float64(BUCKET_SIZE) > 0.75 {
		NEW_BUCKET_SIZE := 2 * BUCKET_SIZE
		new_table := make([][]Entry, NEW_BUCKET_SIZE)
		for _, bucket := range table {
			for _, entry := range bucket {
				new_bucket := int(hash(entry.key) % uint64(NEW_BUCKET_SIZE)) //bucket shd always be int
				new_table[new_bucket] = append(new_table[new_bucket], entry)
			}
		}
		table = new_table
		BUCKET_SIZE = NEW_BUCKET_SIZE
	}
}

func rebuild(file *os.File) {
	//rebuilds the has table when u restart the program so that data isnt lost

	scanner := bufio.NewScanner(file) //obj

	for scanner.Scan() {
		line := scanner.Text()        //gets each line
		parts := strings.Fields(line) //each word of the line (split by spaces)
		var operation, key, val string
		if len(parts) == 3 {
			operation = parts[0]
			key = parts[1]
			val = parts[2]

		} else if len(parts) == 2 {
			operation = parts[0]
			key = parts[1]
		}
		if operation == "PUT" {
			PUT(key, val)
		} else if operation == "DELETE" {
			DELETE(key)
		}
	}
}

// func write_compaction(file *os.File, table [][]Entry) *os.File {
// 	//table is [][]ENtry type
// 	file1, err := os.OpenFile(
// 		"new_data.log",
// 		os.O_TRUNC|os.O_RDWR|os.O_CREATE, //truncation mode - clear all data b4 opening it, create if not exist, read write mode
// 		0644,                             //normal permissions
// 	)

// 	if err != nil {
// 		fmt.Println("error openning file!")
// 		return nil
// 	}
// 	for _, bucket := range table {
// 		for _, entry := range bucket {
// 			file1.WriteString("PUT " + entry.key + " " + entry.val + "\n")
// 		}
// 	}

// 	file1.Close()
// 	file.Close()

// 	os.Rename("new_data.log", "data.log")

// 	file, err = os.OpenFile(
// 		"data.log",
// 		os.O_APPEND|os.O_RDWR,
// 		0644,
// 	)

// 	if err != nil {
// 		fmt.Println("error reopening data.log!")
// 		return nil
// 	}

// 	return file
// }

func sstable(file *os.File) {
	if WAL_ENTRIES < 1000 {
		return
	}

	var entries []Entry
	for _, bucket := range table {
		for _, entry := range bucket {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].key < entries[j].key
	})

	file_name := fmt.Sprintf("sstable_%d.log", SSTable_index)
	file1, err := os.OpenFile(
		file_name,
		os.O_TRUNC|os.O_CREATE|os.O_RDWR,
		0644,
	)
	if err != nil {
		fmt.Println("Error creating sstable file")
		return
	}

	file_name1 := fmt.Sprintf("sstable_%d.index", SSTable_index)
	file2, err := os.OpenFile(
		file_name1,
		os.O_TRUNC|os.O_CREATE|os.O_RDWR,
		0644,
	)
	if err != nil {
		fmt.Println("Error creating sstabl.index file for sparse indexing")
		file2.Close()
		return
	}
	for index, entry := range entries {

		if index%10 == 0 {
			offset, err := file1.Seek(0, io.SeekCurrent)
			if err != nil {
				fmt.Println("error finding offset for sstable.index file")
				file1.Close()
				file2.Close()
				return
			}
			_, err = file2.WriteString(fmt.Sprintf("%s %d\n", entry.key, offset))
			if err != nil {
				fmt.Println("error writing to sstable.index")
				file1.Close()
				file2.Close()
				return
			}
		}
		_, err := file1.WriteString("PUT " + entry.key + " " + entry.val + "\n")

		if err != nil {
			fmt.Println("Error writing to sstable")
			return
		}
	}
	err = file1.Sync() //makes sure data is flushed
	if err != nil {
		fmt.Println("error flushing data to sstable")
		return
	}
	err1 := file2.Sync()
	if err1 != nil {
		fmt.Println("error flushing data to sstable.index")
		return
	}
	file1.Close()
	file2.Close()
	err = file.Truncate(0) //truncates WAL file to empty it
	//0 means set file size to 0 bytes
	if err != nil {
		fmt.Println("error truncating WAL!")
		return
	}
	file.Seek(0, 0) //move cursorback to start in WAL
	table = make([][]Entry, BUCKET_SIZE)
	SSTable_index++
	ENTRIES = 0
	WAL_ENTRIES = 0

	fmt.Println("MemTable successfully flushed to", file_name)
}

func main() {
	get_sstable_index()

	file, err := os.OpenFile(
		"data.log",
		os.O_APPEND|os.O_RDWR|os.O_CREATE, //append mode, create if not exist, read write mode
		0644,                              //normal permissions
	)

	if err != nil {
		fmt.Println("error openning file!")
		return
	}
	defer file.Close()
	rebuild(file)

	fmt.Println("===============================================")
	fmt.Println("   Welcome to the Basic Key-Value Store in Go")
	fmt.Println("===============================================")
	fmt.Println()
	fmt.Println("This is a basic implementation of a Key-Value Store written in Go.")
	fmt.Println("The store uses a hash table to efficiently store and retrieve key-value pairs.")
	fmt.Println()
	fmt.Println("Available Operations:")
	fmt.Println("1. PUT    - Inserts a new key-value pair or updates the value if the key already exists.")
	fmt.Println("2. GET    - Retrieves the value associated with a given key.")
	fmt.Println("3. DELETE - Removes a key-value pair from the store.")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println(`PUT("name", "Keshav")`)
	fmt.Println(`GET("name")`)
	fmt.Println(`DELETE("name")`)
	fmt.Println()
	fmt.Println("===============================================")

	var n int

	for {
		fmt.Scan(&n)
		if n == 1 {
			var a, b string
			fmt.Println("Enter key: ")
			fmt.Scan(&a)
			fmt.Println("Enter value: ")
			fmt.Scan(&b)
			file.WriteString("PUT " + a + " " + b + "\n")
			WAL_ENTRIES++
			PUT(a, b)
			sstable(file)
			//fmt.Println("Successfully inserted key and val!")
		} else if n == 2 {
			var a string
			fmt.Println("Enter key: ")
			fmt.Scan(&a)
			GET(a)
			fmt.Println("Required key is - ")

		} else if n == 3 {
			var a string
			fmt.Println("Enter key: ")
			fmt.Scan(&a)
			file.WriteString("DELETE " + a + "\n")
			WAL_ENTRIES++
			DELETE(a)
			sstable(file)

		} else if n == 4 {
			fmt.Println("Successfully exited program")
			break
		} else {
			fmt.Println("Invalid option. Try again!")
		}
	}

	// file = write_compaction(file, table)
	// if file == nil {
	// 	fmt.Println("write compaction failed")
	// 	return
	// }

}
