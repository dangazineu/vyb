This module is part of a Golang CLI, and it provides the logic for summarizing folders in an application's, based on their content. 

Given a list of files contained in a root fs.FS, the code will:

## Data Structure
Create a custom tree structure representing folders and files in the hierarchy. The folder nodes are either folders with files directly underneath them, or the folders that combine two or more folder nodes. The string name of each file and folder node is the relative path between the nodes.

For example, given the following set of files:

root/dir1/file1.txt
root/dir1/dir2/file2.txt
root/dir3/dir4/dir5/file3.txt
root/dir3/dir4/dir5/file4.txt
root/dir3/file5.txt

There would be the following nodes:

Node:
  Name: root
  Type: Folder
  Parent: nil

Node:
  Name: dir1
  Type: Folder
  Parent: root

Node:
  Name: file2.txt
  Type: File
  Parent: dir1

Node:
  Name: dir2
  Type: Folder
  Parent: dir1

Node:
  Name: file2.txt
  Type: File
  Parent: dir2

Node:
  Name: dir3
  Type: Folder
  Parent: root

Node:
  Name: dir4/dir5
  Type: Folder
  Parent: dir3

Node:
  Name: file3.txt
  Type: File
  Parent: dir4/dir5

Node:
  Name: file4.txt
  Type: File
  Parent: dir4/dir5

Node:
  Name: file5.txt
  Type: File
  Parent: dir3

# 1 - using "github.com/tiktoken-go/tokenizer", how many tokens are included in every file of 