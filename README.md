# Nova-chat
[![NovaChat, Decentralised, Open-Sourced, Secure, Real-Time](https://pimp-my-readme.webapp.io/pimp-my-readme/wavy-banner?subtitle=Decentralised%2C%20Open-Sourced%2C%20Secure%2C%20Real-Time&title=NovaChat)](https://pimp-my-readme.webapp.io)

## IPFS-Based Chat using Gossipsub Protocol

## Usages
Before accessing NovaChat features, you can either run or build the program. You can read more about it here: https://blog.devgenius.io/go-build-vs-go-run-baa3da9715cc

### Change working directory
Change your working directory to Nova-chat project directory:<br>
`cd Nova-chat`

### Running the program
To run the program with default parameters, use the following command:<br>
`go run .`

### Building the program
To build and run the program, output to *novachat*:<br>
`go build -o novachat`  
`./novachat`

### Showing help menu
To show parameters:<br>
`go run . -h`  

![output for help](readme_resources/help_output.png "Help output")

## Advanced Usage

### Running with custom nickname
To run the program with a custom nickname of *Jason Bourne*:<br>
`go run . -nick=JasonBourne`

### Running with custom topic name
To run the program with a custom topic name of *watermelonbananas*:<br>
`go run . -topic=watermelonbananas`

### Running with HTTP (only for web-ui, HTTP instead of HTTPS)
`go run . -http`


## Novachat user interfaces (UI)
Novachat offers 2 user interfaces, one terminal-based and one web-based.

### Web-UI
The web interface can be run using the following command:<br>
`go run . --webui`
#### Default Webpage
![Web interface](readme_resources/webui.png "Web interface")
#### Webpage with peers
![Web interface with peers](readme_resources/webui-withpeers.png "Web interface with peers")


### Terminal-UI
The terminal interface (default) can be run using the following command:<br>
`go run .`

![Terminal interface](readme_resources/terminalui.png "Terminal interface")
