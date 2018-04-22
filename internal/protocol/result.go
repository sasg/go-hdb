/*
Copyright 2014 SAP SE

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package protocol

import (
	"fmt"
	"io"

	"github.com/SAP/go-hdb/internal/bufio"
)

const (
	resultsetIDSize = 8
)

type columnOptions int8

const (
	coMandatory columnOptions = 0x01
	coOptional  columnOptions = 0x02
)

var columnOptionsText = map[columnOptions]string{
	coMandatory: "mandatory",
	coOptional:  "optional",
}

func (k columnOptions) String() string {
	t := make([]string, 0, len(columnOptionsText))

	for option, text := range columnOptionsText {
		if (k & option) != 0 {
			t = append(t, text)
		}
	}
	return fmt.Sprintf("%v", t)
}

//resultset id
type resultsetID struct {
	id *uint64
}

func (id *resultsetID) kind() partKind {
	return pkResultsetID
}

func (id *resultsetID) size() (int, error) {
	return resultsetIDSize, nil
}

func (id *resultsetID) numArg() int {
	return 1
}

func (id *resultsetID) setNumArg(int) {
	//ignore - always 1
}

func (id *resultsetID) read(rd *bufio.Reader) error {
	_id := rd.ReadUint64()
	*id.id = _id

	if trace {
		outLogger.Printf("resultset id: %d", *id.id)
	}

	return rd.GetError()
}

func (id *resultsetID) write(wr *bufio.Writer) error {
	wr.WriteUint64(*id.id)

	if trace {
		outLogger.Printf("resultset id: %d", *id.id)
	}

	return nil
}

const (
	tableName = iota
	schemaName
	columnName
	columnDisplayName
	maxNames
)

type resultField struct {
	fieldNames    fieldNames
	columnOptions columnOptions
	tc            TypeCode
	fraction      int16
	length        int16
	offsets       [maxNames]uint32
}

func newResultField(fieldNames fieldNames) *resultField {
	return &resultField{fieldNames: fieldNames}
}

func (f *resultField) String() string {
	return fmt.Sprintf("columnsOptions %s typeCode %s fraction %d length %d tablename %s schemaname %s columnname %s columnDisplayname %s",
		f.columnOptions,
		f.tc,
		f.fraction,
		f.length,
		f.fieldNames.name(f.offsets[tableName]),
		f.fieldNames.name(f.offsets[schemaName]),
		f.fieldNames.name(f.offsets[columnName]),
		f.fieldNames.name(f.offsets[columnDisplayName]),
	)
}

// Field interface
func (f *resultField) TypeCode() TypeCode {
	return f.tc
}

// TypeLength returns the type length of the field.
// see https://golang.org/pkg/database/sql/driver/#RowsColumnTypeLength
func (f *resultField) TypeLength() (int64, bool) {
	if f.tc.isVariableLength() {
		return int64(f.length), true
	}
	return 0, false
}

// TypePrecisionScale returns the type precision and scale (decimal types) of the field.
// see https://golang.org/pkg/database/sql/driver/#RowsColumnTypePrecisionScale
func (f *resultField) TypePrecisionScale() (int64, int64, bool) {
	if f.tc.isDecimalType() {
		return int64(f.length), int64(f.fraction), true
	}
	return 0, 0, false
}

// Nullable returns true if the field may be null, false otherwise.
// see https://golang.org/pkg/database/sql/driver/#RowsColumnTypeNullable
func (f *resultField) Nullable() bool {
	return f.columnOptions == coOptional
}

func (f *resultField) In() bool {
	return false
}

func (f *resultField) Out() bool {
	return true
}

func (f *resultField) Name() string {
	return f.fieldNames.name(f.offsets[columnDisplayName])
}

func (f *resultField) lobReader() io.Reader {
	return nil
}

func (f *resultField) SetLobReader(rd io.Reader) error {
	return fmt.Errorf("result field does not support lob readers")
}

//

func (f *resultField) read(rd *bufio.Reader) {
	f.columnOptions = columnOptions(rd.ReadInt8())
	f.tc = TypeCode(rd.ReadInt8())
	f.fraction = rd.ReadInt16()
	f.length = rd.ReadInt16()
	rd.Skip(2) //filler
	for i := 0; i < maxNames; i++ {
		offset := rd.ReadUint32()
		f.offsets[i] = offset
		f.fieldNames.addOffset(offset)
	}
}

//resultset metadata
type resultMetadata struct {
	fieldSet *FieldSet
	numArg   int
}

func (r *resultMetadata) String() string {
	return fmt.Sprintf("result metadata: %s", r.fieldSet.fields)
}

func (r *resultMetadata) kind() partKind {
	return pkResultMetadata
}

func (r *resultMetadata) setNumArg(numArg int) {
	r.numArg = numArg
}

func (r *resultMetadata) read(rd *bufio.Reader) error {

	for i := 0; i < r.numArg; i++ {
		field := newResultField(r.fieldSet.names)
		field.read(rd)
		r.fieldSet.fields[i] = field
	}

	pos := uint32(0)
	for _, offset := range r.fieldSet.names.sortOffsets() {
		if diff := int(offset - pos); diff > 0 {
			rd.Skip(diff)
		}
		b, size := readShortUtf8(rd)
		r.fieldSet.names.setName(offset, string(b))
		pos += uint32(1 + size)
	}

	if trace {
		outLogger.Printf("read %s", r)
	}

	return rd.GetError()
}

//resultset
type resultset struct {
	numArg      int
	fieldSet    *FieldSet
	fieldValues *FieldValues
}

func (r *resultset) String() string {
	return fmt.Sprintf("resultset: %s", r.fieldValues)
}

func (r *resultset) kind() partKind {
	return pkResultset
}

func (r *resultset) setNumArg(numArg int) {
	r.numArg = numArg
}

func (r *resultset) read(rd *bufio.Reader) error {
	if err := r.fieldValues.read(r.numArg, r.fieldSet, rd); err != nil {
		return err
	}
	if trace {
		outLogger.Printf("read %s", r)
	}
	return nil
}
