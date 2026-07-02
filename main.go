package main

import (
	"fmt"
	"hash/fnv"
)

const BUCKET_SIZE = 100

// type <name of struct> struct
type Entry struct {
	key string
	val string
}

var table [BUCKET_SIZE][]Entry

func hash(key string) uint64 {
	hash_obj := fnv.New64a()
	hash_obj.Write([]byte(key))
	return hash_obj.Sum64()
}

func PUT(key1 string, value string) {
	hashed_key := hash(key1)
	bucket := hashed_key % BUCKET_SIZE
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
		fmt.Println("successfully added key-value to bucket!")
	}
}

func GET(key1 string) {
	hashed_key := hash(key1)
	bucket := hashed_key % BUCKET_SIZE

	for index, entry := range table[bucket] {
		if entry.key == key1 {
			fmt.Printf("Value is = %s\n", table[bucket][index].val)
		}
	}
}

func DELETE(key1 string) {
	hashed_key := hash(key1)
	bucket := hashed_key % BUCKET_SIZE

	for index, entry := range table[bucket] {
		if entry.key == key1 {
			table[bucket] = append(table[bucket][:index], table[bucket][index+1:]...)
			fmt.Printf("Value is = %s\n", table[bucket][index].val)
		}
	}
}
func main() {
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

	fmt.Scan(&n)

	if n == 1 {
		var a, b string
		fmt.Println("Enter key: ")
		fmt.Scan(&a)
		fmt.Println("Enter value: ")
		fmt.Scan(&b)
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
		DELETE(a)
		fmt.Println("Successfully deleted key !")
	}
}
