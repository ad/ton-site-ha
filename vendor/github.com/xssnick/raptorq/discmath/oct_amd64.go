//go:build amd64

package discmath

import "unsafe"

var _Mul4bitPreCalc = calcOctMul4bitTable()

func calcOctMul4bitTable() [256][32]uint8 {
	var result [256][32]uint8
	for m := 0; m < 256; m++ {
		for i := 0; i < 16; i++ {
			result[m][i] = _MulPreCalc[m][i]
			result[m][16+i] = _MulPreCalc[m][i<<4]
		}
	}
	return result
}

//go:noescape
func asmSSE2XORBlocks(x, y unsafe.Pointer, blocks int)

//go:noescape
func asmSSSE3MulAdd(x, y unsafe.Pointer, table unsafe.Pointer, blocks int)

func OctVecAdd(x, y []byte) {
	n := len(x)

	// split data to 16 byte blocks
	blocks := n / 16

	// xor blocks using sse2 asm
	if blocks > 0 {
		asmSSE2XORBlocks(
			unsafe.Pointer(&x[0]),
			unsafe.Pointer(&y[0]),
			blocks,
		)
	}

	// xor rest using 64-bit chunks first
	i := blocks * 16
	for ; i+8 <= n; i += 8 {
		*(*uint64)(unsafe.Pointer(&x[i])) ^= *(*uint64)(unsafe.Pointer(&y[i]))
	}
	for ; i < n; i++ {
		x[i] ^= y[i]
	}
}

func OctVecMul(vector []byte, multiplier uint8) {
	table := _MulPreCalc[multiplier]
	for i := 0; i < len(vector); i++ {
		vector[i] = table[vector[i]]
	}
}

func OctVecMulAdd(x, y []byte, multiplier uint8) {
	n := len(x)
	if n == 0 {
		return
	}
	table := _Mul4bitPreCalc[multiplier]
	blocks := n / 16
	if blocks > 0 {
		asmSSSE3MulAdd(
			unsafe.Pointer(&x[0]),
			unsafe.Pointer(&y[0]),
			unsafe.Pointer(&table[0]),
			blocks,
		)
	}
	full := _MulPreCalc[multiplier]
	for i := blocks * 16; i < n; i++ {
		x[i] ^= full[y[i]]
	}
}
