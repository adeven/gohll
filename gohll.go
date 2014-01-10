package gohll

import (
    "fmt"
	"errors"
	"github.com/reusee/mmh3"
    "math"
)

const (
	SPARSE byte = iota
	NORMAL
)

var (
	InvalidPError = errors.New("Invalid value of P, must be 4<=p<=25")
)

func MMH3Hash(value string) uint64 {
	hashBytes := mmh3.Hash128([]byte(value))
	var hash uint64
	for i, value := range hashBytes {
		hash |= uint64(value) << uint(i*8)
	}
	return hash
}

type HLL struct {
	P uint8

	Hasher func(string) uint64

	m1 uint
	m2 uint

	alpha  float64
	format byte

	tempSet          *SparseList
	sparseList       *SparseList
	MaxSparseSetSize int

	registers []uint8
}

func NewHLL(p uint8, maxSparseSetSize int) (*HLL, error) {
	if p < 4 || p > 25 {
		return nil, InvalidPError
	}

	m1 := uint(1 << p)
	m2 := uint(1 << 25)

	var alpha float64
	switch m1 {
	case 16:
		alpha = 0.673
	case 32:
		alpha = 0.697
	case 64:
		alpha = 0.709
	default:
		alpha = 0.7213 / (1 + 1.079/float64(m1))
	}

	format := SPARSE

	tempSet := NewSparseList(p, maxSparseSetSize)
	sparseList := NewSparseList(p, int(m1*6))
	maxSparseSetSize = maxSparseSetSize

	return &HLL{
		P:                p,
		MaxSparseSetSize: maxSparseSetSize,
		Hasher:           MMH3Hash,
		m1:               m1,
		m2:               m2,
		alpha:            alpha,
		format:           format,
		tempSet:          tempSet,
		sparseList:       sparseList,
	}, nil
}

func (h *HLL) Add(value string) {
	hash := h.Hasher(value)
	switch h.format {
	case NORMAL:
		h.addNormal(hash)
	case SPARSE:
		h.addSparse(hash)
	}
}

func (h *HLL) addNormal(hash uint64) {
	index := SliceUint64(hash, 63, 64-h.P)
	w := SliceUint64(hash, 63-h.P, 0) << h.P
	rho := LeadingBitUint64(w)
	if h.registers[index] < rho {
		h.registers[index] = rho
	}
}

func (h *HLL) addSparse(hash uint64) {
	k := EncodeHash(hash, h.P)
	h.tempSet.Add(k)
	if h.tempSet.Full() {
		h.sparseList.Merge(h.tempSet)
		if h.sparseList.Full() {
			h.toNormal()
		}
	}
}

func (h *HLL) toNormal() {
	h.format = NORMAL
	h.registers = make([]uint8, h.m1)
	h.sparseList.Merge(h.tempSet)
	for _, value := range h.sparseList.Data {
		index, rho := DecodeHash(value, h.P)
		if h.registers[index] < rho {
			h.registers[index] = rho
		}
	}
    h.sparseList.Clear()
}

func (h *HLL) Cardinality() float64 {
	var cardinality float64
	switch h.format {
	case NORMAL:
		cardinality = h.cardinalityNormal()
	case SPARSE:
		cardinality = h.cardinalitySparse()
	}
	return cardinality
}

func (h *HLL) cardinalityNormal() float64 {
    var V int
    Etop := h.alpha * float64(h.m1 * h.m1)
    Ebottom := 0.0
    for _, value := range h.registers {
        Ebottom += math.Pow(2, -1.0 * float64(value))
        if V == 0 {
            V += 1
        }
    }
    E := Etop / Ebottom
    var Eprime float64
    if E < 5 * float64(h.m1) {
        Eprime = E - EstimateBias(E, h.P)
    } else {
        Eprime = E
    }

    var H float64
    if V != 0 {
        fmt.Println("H = LC", V)
        H = LinearCounting(h.m1, V)
    } else {
        fmt.Println("H = Eprime")
        H = Eprime
    }

    if H <= Threshold(h.P) {
        fmt.Println("using H")
        return H
    } else {
        fmt.Println("using Eprime")
        return Eprime
    }

}

func (h *HLL) cardinalitySparse() float64 {
	h.sparseList.Merge(h.tempSet)
	return LinearCounting(h.m2, int(h.m2)-h.sparseList.Len())
}
