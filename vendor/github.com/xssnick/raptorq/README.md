# RaptorQ: Go Implementation of RaptorQ FEC

[![Go Reference](https://pkg.go.dev/badge/github.com/xssnick/raptorq.svg)](https://pkg.go.dev/github.com/xssnick/raptorq)
[![License](https://img.shields.io/github/license/xssnick/raptorq)](https://github.com/xssnick/raptorq/blob/main/LICENSE)

## Overview

RaptorQ is a Go implementation of the RaptorQ forward error correction (FEC) algorithm. RaptorQ is an advanced fountain code designed for efficient and reliable data transmission over unreliable or lossy networks like UDP.

This library provides a Go-native implementation of the RaptorQ algorithm, allowing encoding and decoding of data blocks with low computational overhead.

## Features

- **Zero dependencies**: Pure go, can be compiled for any architecture and operation system.
- **Efficient encoding and decoding**: Supports encoding large datasets with low overhead.
- **Flexible symbol size**: Customizable symbol sizes to fit various use cases.
- **Low latency**: Designed for real-time applications.
- **Production ready**: Battle tested in [tonutils-go](https://github.com/xssnick/tonutils-go) and in services based on it

## Installation

To install the package, use `go get`:

```sh
go get github.com/xssnick/raptorq
```

Then import it in your Go project:

```go
import "github.com/xssnick/raptorq"
```

## Usage

#### Encode
```go
payload := strings.Repeat("Бу! Испугался? "+
  "Не бойся, я друг, я тебя не обижу. "+
  "Иди сюда, иди ко мне, сядь рядом со мной, посмотри мне в глаза. "+
  "Ты видишь меня? Я тоже тебя вижу. "+
  "Давай смотреть друг на друга до тех пор, пока наши глаза не устанут. "+
  "Ты не хочешь? Почему? Что-то не так?\n", 30)

enc, err := NewRaptorQ(768).CreateEncoder([]byte(payload))
if err != nil {
  panic(err)
}

var i uint32
// base symbols +5% recovery
for i = 0; i < enc.BaseSymbolsNum()+enc.BaseSymbolsNum()/20; i++ {
  send(enc.GenSymbol(i))
}

// sending additional recovery symbols till peer accept payload
for !isAcceptedByPeer() {
  time.Sleep(time.Millisecond * 100)
  send(enc.GenSymbol(i))
  i++
}

println("Delivered")
```

#### Decode
```go
dec, err := NewRaptorQ(768).CreateDecoder(payloadLen)
if err != nil {
  panic(err)
}

for {
  id, data := recvSymbol()
  canTryDecode, err := dec.AddSymbol(id, data)
  if err != nil {
    panic(err)
  }

  if canTryDecode {
    success, result, err := dec.Decode()
    if err != nil {
      panic(err)
    }

    if success {
      sendAccepted()

      println(string(result))
      break
    }
  }
}
```

See `solver_test.go` for more examples

