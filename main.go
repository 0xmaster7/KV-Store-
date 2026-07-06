package main

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
)

var BUCKET_SIZE = 100
var ENTRIES = 0

// type <name of struct> struct
type Entry struct {
	key string
	val string
}

var table = make([][]Entry, BUCKET_SIZE)

func hash(key string) uint64 {
	hash_obj := fnv.New64a()
	hash_obj.Write([]byte(key))
	return hash_obj.Sum64()
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

func GET(key1 string) {
	hashed_key := hash(key1)
	bucket := int(hashed_key % uint64(BUCKET_SIZE))

	for index, entry := range table[bucket] {
		if entry.key == key1 {
			fmt.Printf("Value is = %s\n", table[bucket][index].val)
		}
	}
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

func write_compaction(file *os.File, table [][]Entry) *os.File {
	//table is [][]ENtry type
	file1, err := os.OpenFile(
		"new_data.log",
		os.O_TRUNC|os.O_RDWR|os.O_CREATE, //truncation mode - clear all data b4 opening it, create if not exist, read write mode
		0644,                             //normal permissions
	)

	if err != nil {
		fmt.Println("error openning file!")
		return nil
	}
	for _, bucket := range table {
		for _, entry := range bucket {
			file1.WriteString("PUT " + entry.key + " " + entry.val + "\n")
		}
	}

	file1.Close()
	file.Close()

	os.Rename("new_data.log", "data.log")

	file, err = os.OpenFile(
		"data.log",
		os.O_APPEND|os.O_RDWR,
		0644,
	)

	if err != nil {
		fmt.Println("error reopening data.log!")
		return nil
	}

	return file
}
func main() {

	file, err := os.OpenFile(
		"data.log",
		os.O_APPEND|os.O_RDWR|os.O_CREATE, //append mode, create if not exist, read write mode
		0644,                              //normal permissions
	)

	if err != nil {
		fmt.Println("error openning file!")
		return
	}

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
			PUT(a, b)
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
			DELETE(a)

		} else if n == 4 {
			fmt.Println("Successfully exited program")
			break
		} else {
			fmt.Println("Invalid option. Try again!")
		}
	}

	file = write_compaction(file, table)
	if file == nil {
		fmt.Println("write compaction failed")
		return
	}

	defer file.Close()
}
