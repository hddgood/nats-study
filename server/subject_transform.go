// Copyright 2023 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
)

var commaSeparatorRegEx = regexp.MustCompile(`,\s*`)
var partitionMappingFunctionRegEx = regexp.MustCompile(`{{\s*[pP]artition\s*\((.*)\)\s*}}`)
var wildcardMappingFunctionRegEx = regexp.MustCompile(`{{\s*[wW]ildcard\s*\((.*)\)\s*}}`)
var splitFromLeftMappingFunctionRegEx = regexp.MustCompile(`{{\s*[sS]plit[fF]rom[lL]eft\s*\((.*)\)\s*}}`)
var splitFromRightMappingFunctionRegEx = regexp.MustCompile(`{{\s*[sS]plit[fF]rom[rR]ight\s*\((.*)\)\s*}}`)
var sliceFromLeftMappingFunctionRegEx = regexp.MustCompile(`{{\s*[sS]lice[fF]rom[lL]eft\s*\((.*)\)\s*}}`)
var sliceFromRightMappingFunctionRegEx = regexp.MustCompile(`{{\s*[sS]lice[fF]rom[rR]ight\s*\((.*)\)\s*}}`)
var splitMappingFunctionRegEx = regexp.MustCompile(`{{\s*[sS]plit\s*\((.*)\)\s*}}`)

// Enum for the subject mapping subjectTransform function types
const (
	NoTransform int16 = iota
	BadTransform
	Partition
	Wildcard
	SplitFromLeft
	SplitFromRight
	SliceFromLeft
	SliceFromRight
	Split
)

// Transforms for arbitrarily mapping subjects from one to another for maps, tees and filters.
// These can also be used for proper mapping on wildcard exports/imports.
// These will be grouped and caching and locking are assumed to be in the upper layers.
type subjectTransform struct {
	src, dest            string
	dtoks                []string // destination tokens
	stoks                []string // source tokens
	dtokmftypes          []int16  // destination token mapping function types
	dtokmftokindexesargs [][]int  // destination token mapping function array of source token index arguments
	dtokmfintargs        []int32  // destination token mapping function int32 arguments
	dtokmfstringargs     []string // destination token mapping function string arguments
}

// SubjectTransformer transforms subjects using mappings
//
// This API is not part of the public API and not subject to SemVer protections
type SubjectTransformer interface {
	// TODO(dlc) - We could add in client here to allow for things like foo -> foo.$ACCOUNT
	Match(string) (string, error)
	TransformSubject(subject string) string
	TransformTokenizedSubject(tokens []string) string
}

func NewSubjectTransform(src, dest string) (*subjectTransform, error) {
	// No source given is equivalent to the source being ">"
	if src == _EMPTY_ {
		src = fwcs
	}

	// Both entries need to be valid subjects.
	sv, stokens, npwcs, hasFwc := subjectInfo(src)
	dv, dtokens, dnpwcs, dHasFwc := subjectInfo(dest)

	// Make sure both are valid, match fwc if present and there are no pwcs in the dest subject.
	if !sv || !dv || dnpwcs > 0 || hasFwc != dHasFwc {
		return nil, ErrBadSubject
	}

	var dtokMappingFunctionTypes []int16
	var dtokMappingFunctionTokenIndexes [][]int
	var dtokMappingFunctionIntArgs []int32
	var dtokMappingFunctionStringArgs []string

	// If the src has partial wildcards then the dest needs to have the token place markers.
	if npwcs > 0 || hasFwc {
		// We need to count to make sure that the dest has token holders for the pwcs.
		sti := make(map[int]int)
		for i, token := range stokens {
			if len(token) == 1 && token[0] == pwc {
				sti[len(sti)+1] = i
			}
		}

		nphs := 0
		for _, token := range dtokens {
			tranformType, transformArgWildcardIndexes, transfomArgInt, transformArgString, err := indexPlaceHolders(token)
			if err != nil {
				return nil, err
			}

			if tranformType == NoTransform {
				dtokMappingFunctionTypes = append(dtokMappingFunctionTypes, NoTransform)
				dtokMappingFunctionTokenIndexes = append(dtokMappingFunctionTokenIndexes, []int{-1})
				dtokMappingFunctionIntArgs = append(dtokMappingFunctionIntArgs, -1)
				dtokMappingFunctionStringArgs = append(dtokMappingFunctionStringArgs, _EMPTY_)
			} else {
				nphs++
				// Now build up our runtime mapping from dest to source tokens.
				var stis []int
				for _, wildcardIndex := range transformArgWildcardIndexes {
					if wildcardIndex > npwcs {
						return nil, &mappingDestinationErr{fmt.Sprintf("%s: [%d]", token, wildcardIndex), ErrorMappingDestinationFunctionWildcardIndexOutOfRange}
					}
					stis = append(stis, sti[wildcardIndex])
				}
				dtokMappingFunctionTypes = append(dtokMappingFunctionTypes, tranformType)
				dtokMappingFunctionTokenIndexes = append(dtokMappingFunctionTokenIndexes, stis)
				dtokMappingFunctionIntArgs = append(dtokMappingFunctionIntArgs, transfomArgInt)
				dtokMappingFunctionStringArgs = append(dtokMappingFunctionStringArgs, transformArgString)

			}
		}
		if nphs < npwcs {
			// not all wildcards are being used in the destination
			return nil, &mappingDestinationErr{dest, ErrMappingDestinationNotUsingAllWildcards}
		}
	}

	return &subjectTransform{
		src:                  src,
		dest:                 dest,
		dtoks:                dtokens,
		stoks:                stokens,
		dtokmftypes:          dtokMappingFunctionTypes,
		dtokmftokindexesargs: dtokMappingFunctionTokenIndexes,
		dtokmfintargs:        dtokMappingFunctionIntArgs,
		dtokmfstringargs:     dtokMappingFunctionStringArgs,
	}, nil
}

func getMappingFunctionArgs(functionRegEx *regexp.Regexp, token string) []string {
	commandStrings := functionRegEx.FindStringSubmatch(token)
	if len(commandStrings) > 1 {
		return commaSeparatorRegEx.Split(commandStrings[1], -1)
	}
	return nil
}

// Helper for mapping functions that take a wildcard index and an integer as arguments
func transformIndexIntArgsHelper(token string, args []string, transformType int16) (int16, []int, int32, string, error) {
	if len(args) < 2 {
		return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionNotEnoughArguments}
	}
	if len(args) > 2 {
		return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionTooManyArguments}
	}
	i, err := strconv.Atoi(strings.Trim(args[0], " "))
	if err != nil {
		return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionInvalidArgument}
	}
	mappingFunctionIntArg, err := strconv.Atoi(strings.Trim(args[1], " "))
	if err != nil {
		return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionInvalidArgument}
	}

	return transformType, []int{i}, int32(mappingFunctionIntArg), _EMPTY_, nil
}

// Helper to ingest and index the subjectTransform destination token (e.g. $x or {{}}) in the token
// returns a transformation type, and three function arguments: an array of source subject token indexes,
// and a single number (e.g. number of partitions, or a slice size), and a string (e.g.a split delimiter)
func indexPlaceHolders(token string) (int16, []int, int32, string, error) {
	length := len(token)
	if length > 1 {
		// old $1, $2, etc... mapping format still supported to maintain backwards compatibility
		if token[0] == '$' { // simple non-partition mapping
			tp, err := strconv.Atoi(token[1:])
			if err != nil {
				// other things rely on tokens starting with $ so not an error just leave it as is
				return NoTransform, []int{-1}, -1, _EMPTY_, nil
			}
			return Wildcard, []int{tp}, -1, _EMPTY_, nil
		}

		// New 'mustache' style mapping
		if length > 4 && token[0] == '{' && token[1] == '{' && token[length-2] == '}' && token[length-1] == '}' {
			// wildcard(wildcard token index) (equivalent to $)
			args := getMappingFunctionArgs(wildcardMappingFunctionRegEx, token)
			if args != nil {
				if len(args) == 1 && args[0] == _EMPTY_ {
					return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionNotEnoughArguments}
				}
				if len(args) == 1 {
					tokenIndex, err := strconv.Atoi(strings.Trim(args[0], " "))
					if err != nil {
						return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionInvalidArgument}
					}
					return Wildcard, []int{tokenIndex}, -1, _EMPTY_, nil
				} else {
					return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionTooManyArguments}
				}
			}

			// partition(number of partitions, token1, token2, ...)
			args = getMappingFunctionArgs(partitionMappingFunctionRegEx, token)
			if args != nil {
				if len(args) < 2 {
					return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionNotEnoughArguments}
				}
				if len(args) >= 2 {
					mappingFunctionIntArg, err := strconv.Atoi(strings.Trim(args[0], " "))
					if err != nil {
						return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionInvalidArgument}
					}
					var numPositions = len(args[1:])
					tokenIndexes := make([]int, numPositions)
					for ti, t := range args[1:] {
						i, err := strconv.Atoi(strings.Trim(t, " "))
						if err != nil {
							return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionInvalidArgument}
						}
						tokenIndexes[ti] = i
					}

					return Partition, tokenIndexes, int32(mappingFunctionIntArg), _EMPTY_, nil
				}
			}

			// SplitFromLeft(token, position)
			args = getMappingFunctionArgs(splitFromLeftMappingFunctionRegEx, token)
			if args != nil {
				return transformIndexIntArgsHelper(token, args, SplitFromLeft)
			}

			// SplitFromRight(token, position)
			args = getMappingFunctionArgs(splitFromRightMappingFunctionRegEx, token)
			if args != nil {
				return transformIndexIntArgsHelper(token, args, SplitFromRight)
			}

			// SliceFromLeft(token, position)
			args = getMappingFunctionArgs(sliceFromLeftMappingFunctionRegEx, token)
			if args != nil {
				return transformIndexIntArgsHelper(token, args, SliceFromLeft)
			}

			// SliceFromRight(token, position)
			args = getMappingFunctionArgs(sliceFromRightMappingFunctionRegEx, token)
			if args != nil {
				return transformIndexIntArgsHelper(token, args, SliceFromRight)
			}

			// split(token, deliminator)
			args = getMappingFunctionArgs(splitMappingFunctionRegEx, token)
			if args != nil {
				if len(args) < 2 {
					return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionNotEnoughArguments}
				}
				if len(args) > 2 {
					return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionTooManyArguments}
				}
				i, err := strconv.Atoi(strings.Trim(args[0], " "))
				if err != nil {
					return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrorMappingDestinationFunctionInvalidArgument}
				}
				if strings.Contains(args[1], " ") || strings.Contains(args[1], tsep) {
					return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token: token, err: ErrorMappingDestinationFunctionInvalidArgument}
				}

				return Split, []int{i}, -1, args[1], nil
			}

			return BadTransform, []int{}, -1, _EMPTY_, &mappingDestinationErr{token, ErrUnknownMappingDestinationFunction}
		}
	}
	return NoTransform, []int{-1}, -1, _EMPTY_, nil
}

// Helper function to tokenize subjects with partial wildcards into formal transform destinations.
// e.g. "foo.*.*" -> "foo.$1.$2"
func transformTokenize(subject string) string {
	// We need to make the appropriate markers for the wildcards etc.
	i := 1
	var nda []string
	for _, token := range strings.Split(subject, tsep) {
		if token == pwcs {
			nda = append(nda, fmt.Sprintf("$%d", i))
			i++
		} else {
			nda = append(nda, token)
		}
	}
	return strings.Join(nda, tsep)
}

// Helper function to go from transform destination to a subject with partial wildcards and ordered list of placeholders
// E.g.:
//
//	"bar" -> "bar", []
//	"foo.$2.$1" -> "foo.*.*", ["$2","$1"]
func transformUntokenize(subject string) (string, []string) {
	var phs []string
	var nda []string

	for _, token := range strings.Split(subject, tsep) {
		if len(token) > 1 && token[0] == '$' && token[1] >= '1' && token[1] <= '9' {
			phs = append(phs, token)
			nda = append(nda, pwcs)
		} else {
			nda = append(nda, token)
		}
	}
	return strings.Join(nda, tsep), phs
}

func tokenizeSubject(subject string) []string {
	// Tokenize the subject.
	tsa := [32]string{}
	tts := tsa[:0]
	start := 0
	for i := 0; i < len(subject); i++ {
		if subject[i] == btsep {
			tts = append(tts, subject[start:i])
			start = i + 1
		}
	}
	tts = append(tts, subject[start:])
	return tts
}

// Match will take a literal published subject that is associated with a client and will match and subjectTransform
// the subject if possible.
//
// This API is not part of the public API and not subject to SemVer protections
func (tr *subjectTransform) Match(subject string) (string, error) {
	// Special case: matches any and no no-op subjectTransform. May not be legal config for some features
	// but specific validations made at subjectTransform create time
	if (tr.src == fwcs || tr.src == _EMPTY_) && (tr.dest == fwcs || tr.dest == _EMPTY_) {
		return subject, nil
	}

	tts := tokenizeSubject(subject)
	if !isValidLiteralSubject(tts) {
		return _EMPTY_, ErrBadSubject
	}

	if (tr.src == _EMPTY_ || tr.src == fwcs) || isSubsetMatch(tts, tr.src) {
		return tr.TransformTokenizedSubject(tts), nil
	}
	return _EMPTY_, ErrNoTransforms
}

// TransformSubject transforms a subject
//
// This API is not part of the public API and not subject to SemVer protection
func (tr *subjectTransform) TransformSubject(subject string) string {
	return tr.TransformTokenizedSubject(tokenizeSubject(subject))
}

func (tr *subjectTransform) getHashPartition(key []byte, numBuckets int) string {
	h := fnv.New32a()
	_, _ = h.Write(key)

	return strconv.Itoa(int(h.Sum32() % uint32(numBuckets)))
}

// Do a subjectTransform on the subject to the dest subject.
func (tr *subjectTransform) TransformTokenizedSubject(tokens []string) string {
	if len(tr.dtokmftypes) == 0 {
		return tr.dest
	}

	var b strings.Builder

	// We need to walk destination tokens and create the mapped subject pulling tokens or mapping functions
	li := len(tr.dtokmftypes) - 1
	for i, mfType := range tr.dtokmftypes {
		if mfType == NoTransform {
			// Break if fwc
			if len(tr.dtoks[i]) == 1 && tr.dtoks[i][0] == fwc {
				break
			}
			b.WriteString(tr.dtoks[i])
		} else {
			switch mfType {
			case Partition:
				var (
					_buffer       [64]byte
					keyForHashing = _buffer[:0]
				)
				for _, sourceToken := range tr.dtokmftokindexesargs[i] {
					keyForHashing = append(keyForHashing, []byte(tokens[sourceToken])...)
				}
				b.WriteString(tr.getHashPartition(keyForHashing, int(tr.dtokmfintargs[i])))
			case Wildcard: // simple substitution
				b.WriteString(tokens[tr.dtokmftokindexesargs[i][0]])
			case SplitFromLeft:
				sourceToken := tokens[tr.dtokmftokindexesargs[i][0]]
				sourceTokenLen := len(sourceToken)
				position := int(tr.dtokmfintargs[i])
				if position > 0 && position < sourceTokenLen {
					b.WriteString(sourceToken[:position])
					b.WriteString(tsep)
					b.WriteString(sourceToken[position:])
				} else { // too small to split at the requested position: don't split
					b.WriteString(sourceToken)
				}
			case SplitFromRight:
				sourceToken := tokens[tr.dtokmftokindexesargs[i][0]]
				sourceTokenLen := len(sourceToken)
				position := int(tr.dtokmfintargs[i])
				if position > 0 && position < sourceTokenLen {
					b.WriteString(sourceToken[:sourceTokenLen-position])
					b.WriteString(tsep)
					b.WriteString(sourceToken[sourceTokenLen-position:])
				} else { // too small to split at the requested position: don't split
					b.WriteString(sourceToken)
				}
			case SliceFromLeft:
				sourceToken := tokens[tr.dtokmftokindexesargs[i][0]]
				sourceTokenLen := len(sourceToken)
				sliceSize := int(tr.dtokmfintargs[i])
				if sliceSize > 0 && sliceSize < sourceTokenLen {
					for i := 0; i+sliceSize <= sourceTokenLen; i += sliceSize {
						if i != 0 {
							b.WriteString(tsep)
						}
						b.WriteString(sourceToken[i : i+sliceSize])
						if i+sliceSize != sourceTokenLen && i+sliceSize+sliceSize > sourceTokenLen {
							b.WriteString(tsep)
							b.WriteString(sourceToken[i+sliceSize:])
							break
						}
					}
				} else { // too small to slice at the requested size: don't slice
					b.WriteString(sourceToken)
				}
			case SliceFromRight:
				sourceToken := tokens[tr.dtokmftokindexesargs[i][0]]
				sourceTokenLen := len(sourceToken)
				sliceSize := int(tr.dtokmfintargs[i])
				if sliceSize > 0 && sliceSize < sourceTokenLen {
					remainder := sourceTokenLen % sliceSize
					if remainder > 0 {
						b.WriteString(sourceToken[:remainder])
						b.WriteString(tsep)
					}
					for i := remainder; i+sliceSize <= sourceTokenLen; i += sliceSize {
						b.WriteString(sourceToken[i : i+sliceSize])
						if i+sliceSize < sourceTokenLen {
							b.WriteString(tsep)
						}
					}
				} else { // too small to slice at the requested size: don't slice
					b.WriteString(sourceToken)
				}
			case Split:
				sourceToken := tokens[tr.dtokmftokindexesargs[i][0]]
				splits := strings.Split(sourceToken, tr.dtokmfstringargs[i])
				for j, split := range splits {
					if split != _EMPTY_ {
						b.WriteString(split)
					}
					if j < len(splits)-1 && splits[j+1] != _EMPTY_ && !(j == 0 && split == _EMPTY_) {
						b.WriteString(tsep)
					}
				}
			}
		}

		if i < li {
			b.WriteByte(btsep)
		}
	}

	// We may have more source tokens available. This happens with ">".
	if tr.dtoks[len(tr.dtoks)-1] == fwcs {
		for sli, i := len(tokens)-1, len(tr.stoks)-1; i < len(tokens); i++ {
			b.WriteString(tokens[i])
			if i < sli {
				b.WriteByte(btsep)
			}
		}
	}
	return b.String()
}

// Reverse a subjectTransform.
func (tr *subjectTransform) reverse() *subjectTransform {
	if len(tr.dtokmftokindexesargs) == 0 {
		rtr, _ := NewSubjectTransform(tr.dest, tr.src)
		return rtr
	}
	// If we are here we need to dynamically get the correct reverse
	// of this subjectTransform.
	nsrc, phs := transformUntokenize(tr.dest)
	var nda []string
	for _, token := range tr.stoks {
		if token == pwcs {
			if len(phs) == 0 {
				// TODO(dlc) - Should not happen
				return nil
			}
			nda = append(nda, phs[0])
			phs = phs[1:]
		} else {
			nda = append(nda, token)
		}
	}
	ndest := strings.Join(nda, tsep)
	rtr, _ := NewSubjectTransform(nsrc, ndest)
	return rtr
}
