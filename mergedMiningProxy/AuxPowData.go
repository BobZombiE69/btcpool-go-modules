package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"

	"merkle-tree-and-bitcoin/hash"
	"merkle-tree-and-bitcoin/merkle"
)

// AuxMerkleBranch merged mining Merkle Branch
type AuxMerkleBranch struct {
	branchs  []hash.Byte32
	sideMask uint32
}

// AuxPowData Auxiliary workload data
type AuxPowData struct {
	coinbaseTxn      []byte
	blockHash        hash.Byte32
	coinbaseBranch   AuxMerkleBranch
	blockchainBranch AuxMerkleBranch
	parentBlock      []byte
}

// ParseAuxPowData Parsing auxiliary workload data
/*
<https://en.bitcoin.it/wiki/Merged_mining_specification#Aux_proof-of-work_block>

 ? coinbase_txn         txn             Coinbase transaction that is in the parent block, linking the AuxPOW block to its parent block.
32 block_hash           char[32]        Hash of the parent_block header.
 ? coinbase_branch      Merkle branch   The merkle branch linking the coinbase_txn to the parent block's merkle_root.
 ? blockchain_branch    Merkle branch   The merkle branch linking this auxiliary blockchain to the others,
                                        when used in a merged mining setup with multiple auxiliary chains.
80 parent_block         Block header    Parent block header.
*/
func ParseAuxPowData(dataHex string, chainType string) (auxPowData *AuxPowData, err error) {
	auxPowData = new(AuxPowData)

	data, err := hex.DecodeString(dataHex)
	if err != nil {
		return
	}

	if len(data) <= 80 {
		err = errors.New("AuxPowData should be more than 80 bytes")
		return
	}

	// 80 bytes of parent block header
	auxPowData.parentBlock = make([]byte, 80)
	copy(auxPowData.parentBlock, data[len(data)-80:])

//Because parsing coinbase_txn is very difficult, and its exact length cannot be easily obtained,
//So I decided to calculate block_hash first, and then find the hash from the byte stream to determine the length of coinbase_txn.
	if chainType == "LTC" {
		scryptKey, errScrypt := Scrypt(auxPowData.parentBlock)
		if errScrypt != nil {
			err = errors.New("Scrypt parentBlock failed")
			return
		}
		auxPowData.blockHash.Assign(scryptKey)

	} else {
		auxPowData.blockHash = hash.Hash(auxPowData.parentBlock)
		// The default endianness of BTCPool is big-endian
		auxPowData.blockHash = auxPowData.blockHash.Reverse()
	}


	// found from the byte stream block_hash To determine coinbase_txn length
	index := bytes.Index(data, auxPowData.blockHash[:])
	if index == -1 {
		/* can't find it, try little-endian
		* <https://en.bitcoin.it/wiki/Merged_mining_specification#Aux_proof-of-work_block>
		* Note that the block_hash element is not needed as you have the full parent_block header element
		* and can calculate the hash from that. The current Namecoin client doesn't check this field for
		* validity, and as such some AuxPOW blocks have it little-endian, and some have it big-endian.
		 */
		auxPowData.blockHash = auxPowData.blockHash.Reverse()
		index = bytes.Index(data, auxPowData.blockHash[:])
		if index == -1 {
			err = errors.New("cannot found blockHash " + auxPowData.blockHash.Hex() + " from AuxPowData " + dataHex)
			return
		}

	}

	// index is numerically equal to the length of coinbase_txn
	auxPowData.coinbaseTxn = make([]byte, index)
	copy(auxPowData.coinbaseTxn, data[0:])

	// jump over block_hash
	index += 32

	// coinbaseBranchSize 为变长整数 <https://en.bitcoin.it/wiki/Protocol_documentation#Variable_length_integer> ，
	// But it is unlikely to exceed 0xFD. So let's say coinbaseBranchSize is only one byte.
	coinbaseBranchSize := int(data[index])
	index++

	// read coinbase branch
	auxPowData.coinbaseBranch.branchs = make([]hash.Byte32, coinbaseBranchSize)
	for i := 0; i < coinbaseBranchSize; i++ {
		copy(auxPowData.coinbaseBranch.branchs[i][:], data[index:])
		index += 32
	}

	// read coinbase branch of side mask
	sideMask := make([]byte, 4)
	copy(sideMask, data[index:])
	auxPowData.coinbaseBranch.sideMask = binary.LittleEndian.Uint32(sideMask)
	index += 4

//blockchainBranchSize is a variable length integer <https://en.bitcoin.it/wiki/Protocol_documentation#Variable_length_integer> ,
//but unlikely to exceed 0xFD. So let's say blockchainBranchSize is only one byte.
	blockchainBranchSize := int(data[index])
	index++

	// read blockchain branch
	auxPowData.blockchainBranch.branchs = make([]hash.Byte32, blockchainBranchSize)
	for i := 0; i < blockchainBranchSize; i++ {
		copy(auxPowData.blockchainBranch.branchs[i][:], data[index:])
		index += 32
	}

	// read blockchain branch of side mask
	sideMask = make([]byte, 4)
	copy(sideMask, data[index:])
	auxPowData.blockchainBranch.sideMask = binary.LittleEndian.Uint32(sideMask)
	index += 4

	// Verify that there is only an 80-byte block header left at the end
	extraDataLen := len(data) - index - 80
	if extraDataLen != 0 {
		err = errors.New("AuxPowData has unexpected data: " + strconv.Itoa(extraDataLen) +
			" bytes between blockchainBranchSideMask and blockHeader")
		return
	}

	// The data is legal and the parsing is complete
	return
}

// ExpandingBlockchainBranch Add currency-specific MerkleBranch to AuxPowData.blockchainBranch
func (auxPowData *AuxPowData) ExpandingBlockchainBranch(extBranch merkle.MerklePath) {
	branch := &auxPowData.blockchainBranch

	extBranchLen := uint(len(extBranch))
	branch.sideMask = branch.sideMask << extBranchLen

	extBranchItems := make([]hash.Byte32, extBranchLen)
	for i := uint(0); i < extBranchLen; i++ {
		extBranchItems[i] = extBranch[i].Hash
		if extBranch[i].UseFirstInConcatenation {
			branch.sideMask = branch.sideMask | (uint32(1) << i)
		}
	}

	branch.branchs = append(extBranchItems, branch.branchs...)
}

// ToBytes Convert AuxPowData to byte stream
func (auxPowData *AuxPowData) ToBytes() (data []byte) {

	// parent coinbase transaction
	data = append(data, auxPowData.coinbaseTxn...)

	// parent block hash
	data = append(data, auxPowData.blockHash[:]...)

	// parent coinbase branch
	data = append(data, byte(len(auxPowData.coinbaseBranch.branchs)))
	for _, branch := range auxPowData.coinbaseBranch.branchs {
		data = append(data, branch[:]...)
	}
	sideMask := make([]byte, 4)
	binary.LittleEndian.PutUint32(sideMask, auxPowData.coinbaseBranch.sideMask)
	data = append(data, sideMask...)

	// merged mining blockchain branch
	data = append(data, byte(len(auxPowData.blockchainBranch.branchs)))
	for _, branch := range auxPowData.blockchainBranch.branchs {
		data = append(data, branch[:]...)
	}
	sideMask = make([]byte, 4)
	binary.LittleEndian.PutUint32(sideMask, auxPowData.blockchainBranch.sideMask)
	data = append(data, sideMask...)

	// parent block header
	data = append(data, auxPowData.parentBlock...)

	return
}

// ToHex Convert AuxPowData to hexadecimal string
func (auxPowData *AuxPowData) ToHex() string {
	return hex.EncodeToString(auxPowData.ToBytes())
}
