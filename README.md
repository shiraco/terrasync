terrasync.go
====

## Requirement

* google developer account / calendar api
* client_secret.json
* config.tml (copy from config.tml.sample & update)

## Usage

run terrasync.exe

## Development

### Environment

```
$ git clone https://github.com/shiraco/terrasync.git
```

### Run

```
$ go run main.go
```

when running first time, create credential file.

~/.credentials/terrasync.json

### Build

For example, windows 32bit, .exe.

```
$ GOOS=windows GOARCH=386 go build -o ./bin/windows386/$1.exe
```

Or,

```
$ ./go_build.sh
```

## Licence

[MIT License](https://github.com/shiraco/terrasync/blob/master/LICENSE)

## Author

[shiraco](https://github.com/shiraco)
