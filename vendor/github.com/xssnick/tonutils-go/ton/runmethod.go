package ton

import (
	"context"
	"errors"
	"fmt"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"
	"math/big"
)

var ErrIncorrectResultType = errors.New("incorrect result type")
var ErrResultIndexOutOfRange = errors.New("result index is out of range")

func init() {
	tl.Register(RunSmcMethod{}, "liteServer.runSmcMethod mode:# id:tonNode.blockIdExt account:liteServer.accountId method_id:long params:bytes = liteServer.RunMethodResult")
	tl.Register(RunMethodResult{}, "liteServer.runMethodResult mode:# id:tonNode.blockIdExt shardblk:tonNode.blockIdExt shard_proof:mode.0?bytes proof:mode.0?bytes state_proof:mode.1?bytes init_c7:mode.3?bytes lib_extras:mode.4?bytes exit_code:int result:mode.2?bytes = liteServer.RunMethodResult")
}

type ExecutionResult struct {
	result []any
}

type RunSmcMethod struct {
	Mode     uint32      `tl:"int"`
	ID       *BlockIDExt `tl:"struct"`
	Account  AccountID   `tl:"struct"`
	MethodID uint64      `tl:"long"`
	Params   []byte      `tl:"bytes"`
}

type RunMethodResult struct {
	Mode       uint32      `tl:"flags"`
	ID         *BlockIDExt `tl:"struct"`
	ShardBlock *BlockIDExt `tl:"struct"`
	ShardProof []byte      `tl:"?0 bytes"`
	Proof      []byte      `tl:"?0 bytes"`
	StateProof []byte      `tl:"?1 bytes"`
	InitC7     []byte      `tl:"?3 bytes"`
	LibExtras  []byte      `tl:"?4 bytes"`
	ExitCode   int32       `tl:"int"`
	Result     []byte      `tl:"?2 bytes"`
}

func NewExecutionResult(data []any) *ExecutionResult {
	return &ExecutionResult{data}
}

func (c *APIClient) RunGetMethod(ctx context.Context, blockInfo *BlockIDExt, addr *address.Address, method string, params ...any) (*ExecutionResult, error) {
	var stack tlb.Stack
	for i := len(params) - 1; i >= 0; i-- {
		// push args in reverse order
		stack.Push(params[i])
	}

	req, err := stack.ToCell()
	if err != nil {
		return nil, fmt.Errorf("build stack err: %w", err)
	}

	mode := uint32(1 << 2)
	if c.proofCheckPolicy != ProofCheckPolicyUnsafe {
		mode |= (1 << 1) | (1 << 0)
	}

	var resp tl.Serializable
	err = c.client.QueryLiteserver(ctx, &RunSmcMethod{
		Mode: mode,
		ID:   blockInfo,
		Account: AccountID{
			Workchain: addr.Workchain(),
			ID:        addr.Data(),
		},
		MethodID: tlb.MethodNameHash(method),
		Params:   req.ToBOCWithFlags(false),
	}, &resp)
	if err != nil {
		return nil, err
	}

	switch t := resp.(type) {
	case RunMethodResult:
		if t.ExitCode != 0 && t.ExitCode != 1 {
			return nil, ContractExecError{
				t.ExitCode,
			}
		}

		resCell, err := cell.FromBOC(t.Result)
		if err != nil {
			return nil, err
		}

		if c.proofCheckPolicy != ProofCheckPolicyUnsafe {
			proof, err := cell.FromBOCMultiRoot(t.Proof)
			if err != nil {
				return nil, err
			}

			var shardProof []*cell.Cell
			var shardHash []byte
			if c.proofCheckPolicy != ProofCheckPolicyUnsafe && addr.Workchain() != address.MasterchainID {
				if len(t.ShardProof) == 0 {
					return nil, fmt.Errorf("liteserver has no proof for this account in a given block, request newer block or disable proof checks")
				}

				shardProof, err = cell.FromBOCMultiRoot(t.ShardProof)
				if err != nil {
					return nil, fmt.Errorf("failed to parse shard proof boc: %w", err)
				}

				if t.ShardBlock == nil || len(t.ShardBlock.RootHash) != 32 {
					return nil, fmt.Errorf("shard block not passed")
				}

				shardHash = t.ShardBlock.RootHash
			}

			shardAcc, balanceInfo, err := CheckAccountStateProof(addr, blockInfo, proof, shardProof, shardHash, c.proofCheckPolicy == ProofCheckPolicyUnsafe)
			if err != nil {
				return nil, fmt.Errorf("failed to check acc state proof: %w", err)
			}
			_, _ = shardAcc, balanceInfo

			stateProofCell, err := cell.FromBOC(t.StateProof)
			if err != nil {
				return nil, err
			}

			_, err = cell.UnwrapProof(stateProofCell, shardAcc.Account.Hash(0))
			if err != nil {
				return nil, fmt.Errorf("failed to match state proof to state hash: %w", err)
			}
			// TODO: when tvm implementation ready - execute code and compare result
		}

		var resStack tlb.Stack
		err = resStack.LoadFromCell(resCell.BeginParse())
		if err != nil {
			return nil, err
		}

		var result []any

		for resStack.Depth() > 0 {
			v, err := resStack.Pop()
			if err != nil {
				return nil, err
			}
			result = append(result, v)
		}

		return NewExecutionResult(result), nil
	case LSError:
		return nil, t
	}
	return nil, errUnexpectedResponse(resp)
}

func (r ExecutionResult) AsTuple() []any {
	return r.result
}

func (r ExecutionResult) Int(index uint) (*big.Int, error) {
	if uint(len(r.result)) <= index {
		return nil, ErrResultIndexOutOfRange
	}

	val, ok := r.result[index].(*big.Int)
	if !ok {
		return nil, ErrIncorrectResultType
	}
	return val, nil
}

func (r ExecutionResult) Cell(index uint) (*cell.Cell, error) {
	if uint(len(r.result)) <= index {
		return nil, ErrResultIndexOutOfRange
	}

	val, ok := r.result[index].(*cell.Cell)
	if !ok {
		return nil, ErrIncorrectResultType
	}
	return val, nil
}

func (r ExecutionResult) Slice(index uint) (*cell.Slice, error) {
	if uint(len(r.result)) <= index {
		return nil, ErrResultIndexOutOfRange
	}

	val, ok := r.result[index].(*cell.Slice)
	if !ok {
		return nil, ErrIncorrectResultType
	}
	return val, nil
}

func (r ExecutionResult) Builder(index uint) (*cell.Builder, error) {
	if uint(len(r.result)) <= index {
		return nil, ErrResultIndexOutOfRange
	}

	val, ok := r.result[index].(*cell.Builder)
	if !ok {
		return nil, ErrIncorrectResultType
	}
	return val, nil
}

func (r ExecutionResult) IsNil(index uint) (bool, error) {
	if uint(len(r.result)) <= index {
		return false, ErrResultIndexOutOfRange
	}

	return r.result[index] == nil, nil
}

func (r ExecutionResult) Tuple(index uint) ([]any, error) {
	if uint(len(r.result)) <= index {
		return nil, ErrResultIndexOutOfRange
	}

	val, ok := r.result[index].([]any)
	if !ok {
		return nil, ErrIncorrectResultType
	}
	return val, nil
}

func (r ExecutionResult) MustCell(index uint) *cell.Cell {
	res, err := r.Cell(index)
	if err != nil {
		panic(err)
	}
	return res
}

func (r ExecutionResult) MustSlice(index uint) *cell.Slice {
	res, err := r.Slice(index)
	if err != nil {
		panic(err)
	}
	return res
}

func (r ExecutionResult) MustBuilder(index uint) *cell.Builder {
	res, err := r.Builder(index)
	if err != nil {
		panic(err)
	}
	return res
}

func (r ExecutionResult) MustInt(index uint) *big.Int {
	res, err := r.Int(index)
	if err != nil {
		panic(err)
	}
	return res
}

func (r ExecutionResult) MustTuple(index uint) []any {
	res, err := r.Tuple(index)
	if err != nil {
		panic(err)
	}
	return res
}

func (r ExecutionResult) MustIsNil(index uint) bool {
	res, err := r.IsNil(index)
	if err != nil {
		panic(err)
	}
	return res
}
