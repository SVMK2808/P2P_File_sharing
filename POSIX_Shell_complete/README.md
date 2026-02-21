# POSIX Shell - Custom C++ Shell Implementation

A custom **POSIX-compliant shell** built entirely from scratch in C++, featuring support for process management, piping, redirection, autocomplete, command history, built-in commands like `cd`, `ls`, `echo`, `pwd`, and more.  
This project was developed with a strong focus on modularity using **Object-Oriented Programming (OOP)** practices.

---

## 🚀 Features

### ✅ Core Shell Capabilities
- Interactive command-line interface
- Built-in command execution:  
  `cd`, `pwd`, `echo`, `history`, `pinfo`, `search`
- Execution of external programs via `execvp`
- Background (`&`) and foreground process support

### 🔄 Input/Output Redirection
- Input redirection using `<`
- Output redirection using `>` and `>>`

### | Pipelining
- Supports commands with pipelines like:  
  `cat file.txt | grep hello | wc -l`
- Pipeline-aware process group management

### 🧠 Signal Handling
- Graceful handling of `Ctrl + C` and `Ctrl + Z`
- Foreground process tracking using `tcsetpgrp()` and process groups

### ⌨️ Command Autocomplete
- File/directory path autocompletion using `TAB`
- Intelligent resolution for single and multiple matches

### 📜 Command History
- `Up/Down` arrow key navigation for previous commands
- Persistent in-session command history

### 🔍 Additional Built-in Commands
- `search`: Recursively search for a file
- `pinfo`: Process information like PID, status, and memory
- `ls`: Full implementation with `-a`, `-l`, and `-al` / `-la` options
- `echo`: Supports single/double quotes and variable expansion like `$HOME`

---

## 🛠 Build & Run Instructions

### 🔧 Dependencies
- A **POSIX-compliant** environment (Linux/macOS)
- **C++17** compatible compiler

### 🧱 Compilation

```bash
 g++ -Iheader_files execution_files/*.cpp Builtin_commands/*.cpp -o my_shell

```
 
 Make sure to include all the .cpp files and directories in your compilation

 
## ▶️ Run

```bash
./my_shell

```
## 📂 Project Structure 

Krishna_POSIX_Shell/
|-- execution_files
    |--Autocompleter.cpp
    |--Executor.cpp
    |--main.cpp
    |--Parser.cpp
    |--Pipeliner.cpp
    |--RedirectionHandler.cpp
    └──Shell.cpp
|-- Builtin_commands
    |--Echo.cpp
    |--historyCommand.cpp
    |--LSCommand.cpp
    |--Pinfo.cpp
    └──SearchCommand.cpp
|-- header_files - has the header files with the same names as all the above files. 

|-- README.md

## 🧪 Example Usages

```bash
> pwd
/home/user/POSIX_Shell

> cd Documents

> echo "Hello World"
Hello World

> ls -la
-rw-r--r--  1 user  staff  1234 Jul 3 14:52 file.txt

> search src main.cpp
Found in: src/main.cpp

> sleep 10 | echo Done &
Started background pipeline with PID: 12345

```

## 💻 Terminal Compatibility
- Designed for Unix - like terminals with **raw mode input** (```bash read() ``` used)
- Compatible with **Mac Terminal** and most **Linux distros**

## ⚙️ Implementation Highlights
- **Object - Oriented Design:** Encapsulated shell components into classes
- **Signal - safe Logic:** Proper handling of job control and signals, although requires a bit more work. 
- **Autocomplete:** Directory/file - aware completions with contextual logic
- **Tokenizer:** Respects quotes in command strings
- **Custom Prompt:** Displays ```username@hostname:path>``` with ~ abbreviation

## 🧑‍💻 Author 
**S V Mohit Kumar**
*Passionate about systems programming, C++ and building developer tools*

## Next Steps 
- Add **persistent command history** saved across sessions
- Implement ```jobs```, ```fg```, ```bg``` commands.
- Add **scripting support** via ```.myshellrc```
