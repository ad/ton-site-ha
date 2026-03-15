package raptorq

import (
	"errors"
	"fmt"
)

type Decoder struct {
	symbolSz uint32
	dataSz   uint32

	fastNum uint32
	slowNum uint32

	fastSymbols []*Symbol
	slowSymbols map[uint32]*Symbol

	pm *raptorParams
}

func (r *RaptorQ) CreateDecoder(dataSize uint32) (*Decoder, error) {
	param, err := r.calcParams(dataSize)
	if err != nil {
		return nil, fmt.Errorf("failed to calc params: %w", err)
	}

	return &Decoder{
		symbolSz:    r.symbolSz,
		pm:          param,
		dataSz:      dataSize,
		fastSymbols: make([]*Symbol, param._K),
		slowSymbols: make(map[uint32]*Symbol),
	}, nil
}

func (d *Decoder) AddSymbol(id uint32, data []byte) (bool, error) {
	if uint32(len(data)) != d.symbolSz {
		return false, fmt.Errorf("incorrect symbol size %d, should be %d", len(data), d.symbolSz)
	}

	if id < d.pm._K {
		// add fast symbol
		if d.fastSymbols[id] != nil {
			return d.fastNum+d.slowNum >= d.pm._K, nil
		}
		cp := make([]byte, d.symbolSz)
		copy(cp, data)
		d.fastSymbols[id] = &Symbol{ID: id, Data: cp}
		d.fastNum++
	} else if _, ok := d.slowSymbols[id]; !ok {
		cp := make([]byte, d.symbolSz)
		copy(cp, data)
		d.slowSymbols[id] = &Symbol{ID: id, Data: cp}
		d.slowNum++
	}

	return d.fastNum+d.slowNum >= d.pm._K, nil
}

func (d *Decoder) FastSymbolsNumRequired() uint32 {
	return d.pm._K
}

func (d *Decoder) Decode() (bool, []byte, error) {
	if d.fastNum+d.slowNum < d.pm._K {
		return false, nil, fmt.Errorf("not enough symbols to decode")
	}

	// Build system for Solve from known symbols (no payload copy).
	sz := d.pm._K + uint32(len(d.slowSymbols))
	if sz < d.pm._KPadded {
		sz = d.pm._KPadded
	}
	toRelax := make([]Symbol, 0, sz)

	// add known symbols
	for i := uint32(0); i < d.pm._K; i++ {
		if s := d.fastSymbols[i]; s != nil {
			toRelax = append(toRelax, *s)
		}
	}

	for k, v := range d.slowSymbols {
		if k >= d.pm._K {
			// add offset for additional symbols
			k = k + d.pm._KPadded - d.pm._K
		}
		toRelax = append(toRelax, Symbol{ID: k, Data: v.Data})
	}

	// add padding empty symbols
	for i := uint32(len(toRelax)); i < d.pm._KPadded; i++ {
		zero := make([]byte, d.symbolSz)
		toRelax = append(toRelax, Symbol{
			ID:   i,
			Data: zero,
		})
	}

	// we have not all fast symbols, try to recover them from slow
	relaxed, err := d.pm.Solve(toRelax)
	if err != nil {
		if errors.Is(err, ErrNotEnoughSymbols) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed to relax known symbols, err: %w", err)
	}

	out := make([]byte, d.pm._K*d.symbolSz)
	for i := uint32(0); i < d.pm._K; i++ {
		off := i * d.symbolSz
		if s := d.fastSymbols[i]; s != nil {
			copy(out[off:off+d.symbolSz], s.Data)
		} else {
			copy(out[off:off+d.symbolSz], d.pm.genSymbol(relaxed, d.symbolSz, i))
		}
	}

	return true, out[:d.dataSz], nil
}
