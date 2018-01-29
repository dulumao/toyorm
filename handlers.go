package toyorm

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

func HandlerPreloadInsertOrSave(option string) func(*Context) error {
	return func(ctx *Context) error {
		for fieldName, preload := range ctx.Brick.OneToOnePreload {
			if preload.IsBelongTo == true {
				mainField, subField := preload.RelationField, preload.SubModel.GetOnePrimary()
				preloadBrick := ctx.Brick.Preload(fieldName)
				subRecords := MakeRecordsWithElem(preload.SubModel, ctx.Result.Records.GetFieldAddressType(fieldName))
				for _, record := range ctx.Result.Records.GetRecords() {
					subRecords.Add(record.FieldAddress(fieldName))
				}
				subCtx := preloadBrick.GetContext(option, subRecords)
				ctx.Result.Preload[fieldName] = subCtx.Result
				if err := subCtx.Next(); err != nil {
					return err
				}
				// set model relation field
				for jdx, record := range ctx.Result.Records.GetRecords() {
					subRecord := subRecords.GetRecord(jdx)
					record.SetField(mainField.Name(), subRecord.Field(subField.Name()))
				}
			}
		}

		if err := ctx.Next(); err != nil {
			return err
		}
		for fieldName, preload := range ctx.Brick.OneToOnePreload {
			if preload.IsBelongTo == false {
				preloadBrick := ctx.Brick.Preload(fieldName)
				mainPos, subPos := preload.Model.GetOnePrimary(), preload.RelationField
				subRecords := MakeRecordsWithElem(preload.SubModel, ctx.Result.Records.GetFieldAddressType(fieldName))
				// set sub model relation field
				for i, record := range ctx.Result.Records.GetRecords() {
					// it means relation field, result[j].LastInsertId() is id value
					subRecords.Add(record.FieldAddress(fieldName))
					if primary := record.Field(mainPos.Name()); primary.IsValid() {
						subRecords.GetRecord(i).SetField(subPos.Name(), primary)
					} else {
						panic("relation field not set")
					}
				}
				subCtx := preloadBrick.GetContext(option, subRecords)
				ctx.Result.Preload[fieldName] = subCtx.Result
				if err := subCtx.Next(); err != nil {
					return err
				}
			}
		}

		// one to many
		for fieldName, preload := range ctx.Brick.OneToManyPreload {
			preloadBrick := ctx.Brick.Preload(fieldName)
			mainField, subField := preload.Model.GetOnePrimary(), preload.RelationField
			elemAddressType := reflect.PtrTo(LoopTypeIndirect(ctx.Result.Records.GetFieldType(fieldName)).Elem())
			subRecords := MakeRecordsWithElem(preload.SubModel, elemAddressType)
			// reset sub model relation field
			for _, record := range ctx.Result.Records.GetRecords() {
				if primary := record.Field(mainField.Name()); primary.IsValid() {
					rField := LoopIndirect(record.Field(fieldName))
					for subi := 0; subi < rField.Len(); subi++ {
						subRecord := subRecords.Add(rField.Index(subi).Addr())
						subRecord.SetField(subField.Name(), primary)
					}
				} else {
					return errors.New("some records have not primary")
				}
			}
			subCtx := preloadBrick.GetContext(option, subRecords)
			ctx.Result.Preload[fieldName] = subCtx.Result
			if err := subCtx.Next(); err != nil {
				return err
			}
		}
		// many to many
		for fieldName, preload := range ctx.Brick.ManyToManyPreload {
			subBrick := ctx.Brick.Preload(fieldName)
			middleBrick := NewToyBrick(ctx.Brick.Toy, preload.MiddleModel).CopyStatus(ctx.Brick)

			mainField, subField := preload.Model.GetOnePrimary(), preload.SubModel.GetOnePrimary()
			elemAddressType := reflect.PtrTo(LoopTypeIndirect(ctx.Result.Records.GetFieldType(fieldName)).Elem())
			subRecords := MakeRecordsWithElem(preload.SubModel, elemAddressType)

			for _, record := range ctx.Result.Records.GetRecords() {
				rField := LoopIndirect(record.Field(fieldName))
				for subi := 0; subi < rField.Len(); subi++ {
					subRecords.Add(rField.Index(subi).Addr())
				}
			}
			subCtx := subBrick.GetContext(option, subRecords)
			ctx.Result.Preload[fieldName] = subCtx.Result
			if err := subCtx.Next(); err != nil {
				return err
			}

			middleRecords := MakeRecordsWithElem(middleBrick.model, middleBrick.model.ReflectType)
			// use to calculate what sub records belong for
			offset := 0
			for _, record := range ctx.Result.Records.GetRecords() {
				primary := record.Field(mainField.Name())
				primary.IsValid()
				if primary.IsValid() == false {
					return errors.New("some records have not primary")
				}
				rField := LoopIndirect(record.Field(fieldName))
				for subi := 0; subi < rField.Len(); subi++ {
					subRecord := subRecords.GetRecord(subi + offset)
					subPrimary := subRecord.Field(subField.Name())
					if subPrimary.IsValid() == false {
						return errors.New("some records have not primary")
					}
					middleRecord := NewRecord(middleBrick.model, reflect.New(middleBrick.model.ReflectType).Elem())
					middleRecord.SetField(preload.RelationField.Name(), primary)
					middleRecord.SetField(preload.SubRelationField.Name(), subPrimary)
					middleRecords.Add(middleRecord.Source())
				}
				offset += rField.Len()
			}
			middleCtx := middleBrick.GetContext(option, middleRecords)
			ctx.Result.MiddleModelPreload[fieldName] = middleCtx.Result
			if err := middleCtx.Next(); err != nil {
				return err
			}
		}
		return nil
	}
}

func HandlerInsertTimeGenerate(ctx *Context) error {
	records := ctx.Result.Records
	createField := ctx.Brick.model.GetFieldWithName("CreatedAt")
	updateField := ctx.Brick.model.GetFieldWithName("UpdatedAt")
	if createField != nil || updateField != nil {
		current := time.Now()
		if createField != nil {
			for _, record := range records.GetRecords() {
				record.SetField(createField.Name(), reflect.ValueOf(current))
			}
		}
		if updateField != nil {
			for _, record := range records.GetRecords() {
				record.SetField(updateField.Name(), reflect.ValueOf(current))
			}
		}
	}
	return nil
}

func HandlerInsert(ctx *Context) error {
	// current insert

	for i, record := range ctx.Result.Records.GetRecords() {
		action := ExecAction{}
		action.Exec = ctx.Brick.InsertExec(record)
		action.Result, action.Error = ctx.Brick.Exec(action.Exec)
		if action.Error == nil {
			// set primary field value if model is autoincrement
			if len(ctx.Brick.model.GetPrimary()) == 1 && ctx.Brick.model.GetOnePrimary().AutoIncrement() == true {
				if lastId, err := action.Result.LastInsertId(); err == nil {
					ctx.Result.Records.GetRecord(i).SetField(ctx.Brick.model.GetOnePrimary().Name(), reflect.ValueOf(lastId))
				} else {
					return errors.New(fmt.Sprintf("get (%s) auto increment  failure reason(%s)", ctx.Brick.model.Name, err))
				}
			}
		}
		ctx.Result.AddExecRecord(action, i)
	}
	return nil
}

func HandlerFind(ctx *Context) error {
	action := QueryAction{
		Exec: ctx.Brick.FindExec(ctx.Result.Records),
	}
	rows, err := ctx.Brick.Query(action.Exec)
	if err != nil {
		action.Error = append(action.Error, err)
		ctx.Result.AddQueryRecord(action)
		return err
	}
	// find current data
	min := ctx.Result.Records.Len()
	for rows.Next() {
		elem := reflect.New(ctx.Result.Records.ElemType()).Elem()
		ctx.Result.Records.Len()
		record := ctx.Result.Records.Add(elem)
		var scanners []interface{}
		for _, field := range ctx.Brick.getScanFields(ctx.Result.Records) {
			value := record.Field(field.Name())
			scanners = append(scanners, value.Addr().Interface())
		}
		err := rows.Scan(scanners...)
		action.Error = append(action.Error, err)
	}
	max := ctx.Result.Records.Len()
	ctx.Result.AddQueryRecord(action, makeRange(min, max)...)
	return nil
}

func HandlerPreloadFind(ctx *Context) error {
	records := ctx.Result.Records
	for fieldName, preload := range ctx.Brick.OneToOnePreload {
		var mainField, subField Field
		// select fields from subtable where ... and subtable.id = table.subtableID
		// select fields from subtable where ... and subtable.tableID = table.ID
		if preload.IsBelongTo {
			mainField, subField = preload.RelationField, preload.SubModel.GetOnePrimary()
		} else {
			mainField, subField = preload.Model.GetOnePrimary(), preload.RelationField
		}
		brick := ctx.Brick.MapPreloadBrick[fieldName]

		keys := reflect.New(reflect.SliceOf(records.GetFieldType(mainField.Name()))).Elem()
		for _, record := range records.GetRecords() {
			keys = SafeAppend(keys, record.Field(mainField.Name()))
		}
		if keys.Len() != 0 {
			// the relation condition should have lowest priority
			brick = brick.Where(ExprIn, subField, keys.Interface()).And().Conditions(brick.Search)
			containerList := reflect.New(reflect.SliceOf(records.GetFieldType(fieldName))).Elem()
			//var preloadRecords ModelRecords
			subCtx, err := brick.find(LoopIndirectAndNew(containerList))
			ctx.Result.Preload[fieldName] = subCtx.Result
			if err != nil {
				return err
			}
			// use to map preload model relation field
			fieldMapKeyType := LoopTypeIndirect(subCtx.Result.Records.GetFieldType(subField.Name()))
			fieldMapType := reflect.MapOf(fieldMapKeyType, records.GetFieldType(fieldName))
			fieldMap := reflect.MakeMap(fieldMapType)
			for i, pRecord := range subCtx.Result.Records.GetRecords() {
				fieldMapKey := LoopIndirect(pRecord.Field(subField.Name()))
				if fieldMapKey.IsValid() {
					fieldMap.SetMapIndex(fieldMapKey, containerList.Index(i))
				}
			}
			for _, record := range records.GetRecords() {
				key := record.Field(mainField.Name())
				if preloadMatchValue := fieldMap.MapIndex(key); preloadMatchValue.IsValid() {
					record.Field(fieldName).Set(preloadMatchValue)
				}
			}
		}
	}
	// one to many
	for fieldName, preload := range ctx.Brick.OneToManyPreload {
		mainField, subField := preload.Model.GetOnePrimary(), preload.RelationField
		brick := ctx.Brick.MapPreloadBrick[fieldName]

		keys := reflect.New(reflect.SliceOf(records.GetFieldType(mainField.Name()))).Elem()
		for _, fieldValue := range records.GetRecords() {
			keys = SafeAppend(keys, fieldValue.Field(mainField.Name()))
		}
		if keys.Len() != 0 {
			// the relation condition should have lowest priority
			brick = brick.Where(ExprIn, subField, keys.Interface()).And().Conditions(brick.Search)
			containerList := reflect.New(records.GetFieldType(fieldName)).Elem()
			//var preloadRecords ModelRecords
			subCtx, err := brick.find(LoopIndirectAndNew(containerList))
			ctx.Result.Preload[fieldName] = subCtx.Result
			if err != nil {
				return err
			}
			// fieldMap:  map[submodel.id]->submodel
			fieldMapKeyType := LoopTypeIndirect(subCtx.Result.Records.GetFieldType(subField.Name()))
			fieldMapValueType := records.GetFieldType(fieldName)
			fieldMapType := reflect.MapOf(fieldMapKeyType, fieldMapValueType)
			fieldMap := reflect.MakeMap(fieldMapType)
			for i, pRecord := range subCtx.Result.Records.GetRecords() {
				fieldMapKey := LoopIndirect(pRecord.Field(subField.Name()))
				if fieldMapKey.IsValid() {
					currentListField := fieldMap.MapIndex(fieldMapKey)
					if currentListField.IsValid() == false {
						currentListField = reflect.MakeSlice(fieldMapValueType, 0, 1)
					}
					fieldMap.SetMapIndex(fieldMapKey, SafeAppend(currentListField, containerList.Index(i)))
				}
			}
			for _, record := range records.GetRecords() {
				key := record.Field(mainField.Name())
				if preloadMatchValue := fieldMap.MapIndex(key); preloadMatchValue.IsValid() {
					record.Field(preload.ContainerField.Name()).Set(preloadMatchValue)
				}
			}
		}
	}
	// many to many
	for fieldName, preload := range ctx.Brick.ManyToManyPreload {
		mainPrimary, subPrimary := preload.Model.GetOnePrimary(), preload.SubModel.GetOnePrimary()
		middleBrick := NewToyBrick(ctx.Brick.Toy, preload.MiddleModel).CopyStatus(ctx.Brick)

		// primaryMap: map[model.id]->the model's ModelRecord
		primaryMap := map[interface{}]ModelRecord{}
		keys := reflect.New(reflect.SliceOf(preload.RelationField.StructField().Type)).Elem()
		for _, record := range records.GetRecords() {
			keys = SafeAppend(keys, record.Field(mainPrimary.Name()))
			primaryMap[record.Field(mainPrimary.Name()).Interface()] = record
		}
		// the relation condition should have lowest priority
		middleBrick = middleBrick.Where(ExprIn, preload.RelationField, keys.Interface()).And().Conditions(middleBrick.Search)
		middleModelElemList := reflect.New(reflect.SliceOf(preload.MiddleModel.ReflectType)).Elem()
		//var middleModelRecords ModelRecords
		middleCtx, err := middleBrick.find(middleModelElemList)
		ctx.Result.MiddleModelPreload[fieldName] = middleCtx.Result
		if err != nil {
			return err
		}
		// middle model records
		if middleCtx.Result.Records.Len() == 0 {
			continue
		}
		// primaryMap

		// subPrimaryMap:  map[submodel.id]->[]the model's ModelRecord
		subPrimaryMap := map[interface{}][]ModelRecord{}
		middlePrimary2Keys := reflect.New(reflect.SliceOf(preload.SubRelationField.StructField().Type)).Elem()
		for _, pRecord := range middleCtx.Result.Records.GetRecords() {
			subPrimaryMapKey := LoopIndirect(pRecord.Field(preload.SubRelationField.Name()))
			subPrimaryMapValue := LoopIndirect(pRecord.Field(preload.RelationField.Name()))
			subPrimaryMap[subPrimaryMapKey.Interface()] =
				append(subPrimaryMap[subPrimaryMapKey.Interface()], primaryMap[subPrimaryMapValue.Interface()])
		}
		for key, _ := range subPrimaryMap {
			middlePrimary2Keys = reflect.Append(middlePrimary2Keys, reflect.ValueOf(key))
		}
		brick := ctx.Brick.MapPreloadBrick[fieldName]
		if middlePrimary2Keys.Len() != 0 {
			// the relation condition should have lowest priority
			brick = brick.Where(ExprIn, subPrimary, middlePrimary2Keys.Interface()).And().Conditions(brick.Search)
			containerField := reflect.New(records.GetFieldType(fieldName)).Elem()
			//var subRecords ModelRecords
			subCtx, err := brick.find(LoopIndirectAndNew(containerField))
			ctx.Result.Preload[fieldName] = subCtx.Result
			if err != nil {
				return err
			}
			for i, subRecord := range subCtx.Result.Records.GetRecords() {
				records := subPrimaryMap[subRecord.Field(subPrimary.Name()).Interface()]
				for _, record := range records {
					record.SetField(fieldName, reflect.Append(record.Field(fieldName), containerField.Index(i)))
				}
			}
		}
	}
	return nil
}

func HandlerUpdateTimeGenerate(ctx *Context) error {
	records := ctx.Result.Records
	if updateField := ctx.Brick.model.GetFieldWithName("UpdatedAt"); updateField != nil {
		current := reflect.ValueOf(time.Now())
		for _, record := range records.GetRecords() {
			record.SetField(updateField.Name(), current)
		}
	}
	return nil
}

func HandlerUpdate(ctx *Context) error {
	for i, record := range ctx.Result.Records.GetRecords() {
		action := ExecAction{Exec: ctx.Brick.UpdateExec(record)}
		action.Result, action.Error = ctx.Brick.Exec(action.Exec)
		ctx.Result.AddExecRecord(action, i)
	}
	return nil
}

// if have not primary ,try to insert
// else try to replace
func HandlerSave(ctx *Context) error {
	for i, record := range ctx.Result.Records.GetRecords() {
		primaryFields := ctx.Brick.model.GetPrimary()
		var tryInsert bool
		for _, primaryField := range primaryFields {
			pkeyFieldValue := record.Field(primaryField.Name())
			if pkeyFieldValue.IsValid() == false || IsZero(pkeyFieldValue) {
				tryInsert = true
				break
			}
		}
		var action ExecAction
		if tryInsert {
			action = ExecAction{}
			action.Exec = ctx.Brick.InsertExec(record)
			action.Result, action.Error = ctx.Brick.Exec(action.Exec)
			if action.Error == nil {
				// set primary field value if model is autoincrement
				if len(ctx.Brick.model.GetPrimary()) == 1 && ctx.Brick.model.GetOnePrimary().AutoIncrement() == true {
					if lastId, err := action.Result.LastInsertId(); err == nil {
						ctx.Result.Records.GetRecord(i).SetField(ctx.Brick.model.GetOnePrimary().Name(), reflect.ValueOf(lastId))
					} else {
						return errors.New(fmt.Sprintf("get (%s) auto increment  failure reason(%s)", ctx.Brick.model.Name, err))
					}
				}
			}
		} else {
			action = ExecAction{}
			action.Exec = ctx.Brick.ReplaceExec(record)
			action.Result, action.Error = ctx.Brick.Exec(action.Exec)
		}
		ctx.Result.AddExecRecord(action, i)
	}
	return nil
}

func HandlerSaveTimeGenerate(ctx *Context) error {
	createdAtField := ctx.Brick.model.GetFieldWithName("CreatedAt")
	// TODO process a exist deleted_at time
	deletedAtField := ctx.Brick.model.GetFieldWithName("DeletedAt")
	now := reflect.ValueOf(time.Now())

	var timeFields []Field
	var defaultFieldValue []reflect.Value
	if createdAtField != nil {

		timeFields = append(timeFields, createdAtField)
		defaultFieldValue = append(defaultFieldValue, now)
	}
	if deletedAtField != nil {
		timeFields = append(timeFields, deletedAtField)
		defaultFieldValue = append(defaultFieldValue, reflect.Zero(deletedAtField.StructField().Type))
	}

	if ctx.Result.Records.Len() > 0 && len(timeFields) > 0 {
		primaryField := ctx.Brick.model.GetOnePrimary()
		brick := ctx.Brick.bindFields(ModeDefault, append([]Field{primaryField}, timeFields...)...)
		primaryKeys := reflect.MakeSlice(reflect.SliceOf(primaryField.StructField().Type), 0, ctx.Result.Records.Len())
		action := QueryAction{}
		var tryFindTimeIndex []int

		for i, record := range ctx.Result.Records.GetRecords() {
			pri := record.Field(primaryField.Name())
			if pri.IsValid() && IsZero(pri) == false {
				primaryKeys = reflect.Append(primaryKeys, pri)
			}
			tryFindTimeIndex = append(tryFindTimeIndex, i)
		}
		if primaryKeys.Len() > 0 {
			action.Exec = brick.Where(ExprIn, primaryField, primaryKeys.Interface()).FindExec(ctx.Result.Records)

			rows, err := brick.Query(action.Exec)
			if err != nil {
				action.Error = append(action.Error, err)
				ctx.Result.AddQueryRecord(action, tryFindTimeIndex...)
				return nil
			}
			var mapElemTypeFields []reflect.StructField
			{
				for _, f := range timeFields {
					mapElemTypeFields = append(mapElemTypeFields, f.StructField())
				}
			}
			mapElemType := reflect.StructOf(mapElemTypeFields)
			primaryKeysMap := reflect.MakeMap(reflect.MapOf(primaryField.StructField().Type, mapElemType))

			// find all createtime
			for rows.Next() {
				id := reflect.New(primaryField.StructField().Type)
				timeFieldValues := reflect.New(mapElemType).Elem()
				scaners := []interface{}{id.Interface()}
				for i := 0; i < timeFieldValues.NumField(); i++ {
					scaners = append(scaners, timeFieldValues.Field(i).Addr().Interface())
				}
				err := rows.Scan(scaners...)
				if err != nil {
					action.Error = append(action.Error, err)
				}
				primaryKeysMap.SetMapIndex(id.Elem(), timeFieldValues)
			}

			ctx.Result.AddQueryRecord(action, tryFindTimeIndex...)
			for _, record := range ctx.Result.Records.GetRecords() {
				pri := record.Field(primaryField.Name())
				fields := primaryKeysMap.MapIndex(pri)
				if fields.IsValid() {
					for i := 0; i < fields.NumField(); i++ {
						field := fields.Field(i)
						if field.IsValid() && IsZero(field) == false {
							record.SetField(timeFields[i].Name(), field)
						} else if IsZero(record.Field(timeFields[i].Name())) {
							record.SetField(timeFields[i].Name(), defaultFieldValue[i])
						}
					}
				} else {
					for i := 0; i < len(timeFields); i++ {
						if IsZero(record.Field(timeFields[i].Name())) {
							record.SetField(timeFields[i].Name(), defaultFieldValue[i])
						}
					}
				}
			}
		} else {
			for _, record := range ctx.Result.Records.GetRecords() {
				for i := 0; i < len(timeFields); i++ {
					if IsZero(record.Field(timeFields[i].Name())) {
						record.SetField(timeFields[i].Name(), defaultFieldValue[i])
					}
				}
			}
		}
	}
	if updateField := ctx.Brick.model.GetFieldWithName("UpdatedAt"); updateField != nil {
		for _, record := range ctx.Result.Records.GetRecords() {
			record.SetField(updateField.Name(), now)
		}
	}
	return nil
}

// preload schedule belongTo -> Next() -> oneToOne -> oneToMany -> manyToMany(sub -> middle)
func HandlerSimplePreload(option string) func(ctx *Context) error {
	return func(ctx *Context) (err error) {
		for fieldName, p := range ctx.Brick.OneToOnePreload {
			if p.IsBelongTo {
				brick := ctx.Brick.MapPreloadBrick[fieldName]
				subCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
				ctx.Result.Preload[fieldName] = subCtx.Result
				if err := subCtx.Next(); err != nil {
					return err
				}
			}
		}
		err = ctx.Next()
		if err != nil {
			return err
		}
		for fieldName, p := range ctx.Brick.OneToOnePreload {
			if p.IsBelongTo == false {
				brick := ctx.Brick.MapPreloadBrick[fieldName]
				subCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
				ctx.Result.Preload[fieldName] = subCtx.Result
				if err := subCtx.Next(); err != nil {
					return err
				}
			}
		}

		for fieldName, _ := range ctx.Brick.OneToManyPreload {
			brick := ctx.Brick.MapPreloadBrick[fieldName]
			subCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
			ctx.Result.Preload[fieldName] = subCtx.Result
			if err := subCtx.Next(); err != nil {
				return err
			}
		}
		for fieldName, preload := range ctx.Brick.ManyToManyPreload {
			{
				brick := ctx.Brick.MapPreloadBrick[fieldName]
				subCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
				ctx.Result.Preload[fieldName] = subCtx.Result
				if err := subCtx.Next(); err != nil {
					return err
				}
			}
			// process middle model
			{
				middleModel := preload.MiddleModel
				brick := NewToyBrick(ctx.Brick.Toy, middleModel).CopyStatus(ctx.Brick)
				middleCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
				ctx.Result.MiddleModelPreload[fieldName] = middleCtx.Result
				if err := middleCtx.Next(); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

// preload schedule oneToOne -> oneToMany -> current model -> manyToMany(sub -> middle) -> Next() -> belongTo
func HandlerDropTablePreload(option string) func(ctx *Context) error {
	return func(ctx *Context) (err error) {
		for fieldName, p := range ctx.Brick.OneToOnePreload {
			if !p.IsBelongTo {
				brick := ctx.Brick.MapPreloadBrick[fieldName]
				subCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
				ctx.Result.Preload[fieldName] = subCtx.Result
				if err := subCtx.Next(); err != nil {
					return err
				}
			}
		}
		for fieldName, _ := range ctx.Brick.OneToManyPreload {
			brick := ctx.Brick.MapPreloadBrick[fieldName]
			subCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
			ctx.Result.Preload[fieldName] = subCtx.Result
			if err := subCtx.Next(); err != nil {
				return err
			}
		}
		for fieldName, preload := range ctx.Brick.ManyToManyPreload {
			// process middle model
			{
				middleModel := preload.MiddleModel
				brick := NewToyBrick(ctx.Brick.Toy, middleModel).CopyStatus(ctx.Brick)
				middleCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
				ctx.Result.MiddleModelPreload[fieldName] = middleCtx.Result
				if err := middleCtx.Next(); err != nil {
					return err
				}
			}
			// process sub model
			{
				brick := ctx.Brick.MapPreloadBrick[fieldName]
				subCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
				ctx.Result.Preload[fieldName] = subCtx.Result
				if err := subCtx.Next(); err != nil {
					return err
				}
			}
		}
		err = ctx.Next()
		if err != nil {
			return err
		}
		for fieldName, p := range ctx.Brick.OneToOnePreload {
			if p.IsBelongTo {
				brick := ctx.Brick.MapPreloadBrick[fieldName]
				subCtx := brick.GetContext(option, MakeRecordsWithElem(brick.model, brick.model.ReflectType))
				ctx.Result.Preload[fieldName] = subCtx.Result
				if err := subCtx.Next(); err != nil {
					return err
				}
			}
		}

		return nil
	}
}

func HandlerCreateTable(ctx *Context) error {
	execs := ctx.Brick.CreateTableExec(ctx.Brick.Toy.Dialect)
	for _, exec := range execs {
		action := ExecAction{Exec: exec}
		action.Result, action.Error = ctx.Brick.Exec(exec)
		ctx.Result.AddExecRecord(action)
	}
	return nil
}

func HandlerExistTableAbort(ctx *Context) error {
	action := QueryAction{}
	action.Exec = ctx.Brick.HasTableExec(ctx.Brick.Toy.Dialect)
	var hasTable bool
	err := ctx.Brick.QueryRow(action.Exec).Scan(&hasTable)
	if err != nil {
		action.Error = append(action.Error, err)
	}
	ctx.Result.AddQueryRecord(action)
	if err != nil || hasTable == true {
		ctx.Abort()
	}

	return nil
}

func HandlerDropTable(ctx *Context) (err error) {
	exec := ctx.Brick.DropTableExec()
	action := ExecAction{Exec: exec}
	action.Result, action.Error = ctx.Brick.Exec(exec)
	ctx.Result.AddExecRecord(action)
	return nil
}

func HandlerNotExistTableAbort(ctx *Context) error {
	action := QueryAction{}
	action.Exec = ctx.Brick.HasTableExec(ctx.Brick.Toy.Dialect)
	var hasTable bool
	err := ctx.Brick.QueryRow(action.Exec).Scan(&hasTable)
	if err != nil {
		action.Error = append(action.Error, err)
	}
	ctx.Result.AddQueryRecord(action)
	if err != nil || hasTable == false {
		ctx.Abort()
	}
	return nil
}

func HandlerPreloadDelete(ctx *Context) error {
	for fieldName, preload := range ctx.Brick.OneToOnePreload {
		if preload.IsBelongTo == false {
			preloadBrick := ctx.Brick.Preload(fieldName)
			subRecords := MakeRecordsWithElem(preload.SubModel, ctx.Result.Records.GetFieldAddressType(fieldName))
			mainSoftDelete := preload.Model.GetFieldWithName("DeletedAt") != nil
			subSoftDelete := preload.SubModel.GetFieldWithName("DeletedAt") != nil
			// set sub model relation field
			for _, record := range ctx.Result.Records.GetRecords() {
				// it means relation field, result[j].LastInsertId() is id value
				subRecords.Add(record.FieldAddress(fieldName))
			}
			// if main model is hard delete need set relationship field set zero if sub model is soft delete
			if mainSoftDelete == false && subSoftDelete == true {
				deletedAtField := preloadBrick.model.GetFieldWithName("DeletedAt")
				preloadBrick = preloadBrick.bindDefaultFields(preload.RelationField, deletedAtField)
			}
			result, err := preloadBrick.deleteWithPrimaryKey(subRecords)
			ctx.Result.Preload[fieldName] = result
			if err != nil {
				return err
			}
		}
	}

	// one to many
	for fieldName, preload := range ctx.Brick.OneToManyPreload {
		preloadBrick := ctx.Brick.Preload(fieldName)
		mainSoftDelete := preload.Model.GetFieldWithName("DeletedAt") != nil
		subSoftDelete := preload.SubModel.GetFieldWithName("DeletedAt") != nil
		elemAddressType := reflect.PtrTo(LoopTypeIndirect(ctx.Result.Records.GetFieldType(fieldName)).Elem())
		subRecords := MakeRecordsWithElem(preload.SubModel, elemAddressType)
		for _, record := range ctx.Result.Records.GetRecords() {
			rField := LoopIndirect(record.Field(fieldName))
			for subi := 0; subi < rField.Len(); subi++ {
				subRecords.Add(rField.Index(subi).Addr())
			}
		}
		// model relationship field set zero
		if mainSoftDelete == false && subSoftDelete == true {
			deletedAtField := preloadBrick.model.GetFieldWithName("DeletedAt")
			preloadBrick = preloadBrick.bindDefaultFields(preload.RelationField, deletedAtField)
		}
		result, err := preloadBrick.deleteWithPrimaryKey(subRecords)
		ctx.Result.Preload[fieldName] = result
		if err != nil {
			return err
		}
	}
	// many to many
	for fieldName, preload := range ctx.Brick.ManyToManyPreload {
		subBrick := ctx.Brick.Preload(fieldName)
		middleBrick := NewToyBrick(ctx.Brick.Toy, preload.MiddleModel).CopyStatus(ctx.Brick)
		mainField, subField := preload.Model.GetOnePrimary(), preload.SubModel.GetOnePrimary()
		mainSoftDelete := preload.Model.GetFieldWithName("DeletedAt") != nil
		subSoftDelete := preload.SubModel.GetFieldWithName("DeletedAt") != nil

		elemAddressType := reflect.PtrTo(LoopTypeIndirect(ctx.Result.Records.GetFieldType(fieldName)).Elem())
		subRecords := MakeRecordsWithElem(preload.SubModel, elemAddressType)

		for _, record := range ctx.Result.Records.GetRecords() {
			rField := LoopIndirect(record.Field(fieldName))
			for subi := 0; subi < rField.Len(); subi++ {
				subRecords.Add(rField.Index(subi).Addr())
			}
		}

		middleRecords := MakeRecordsWithElem(middleBrick.model, middleBrick.model.ReflectType)
		// use to calculate what sub records belong for
		offset := 0
		for _, record := range ctx.Result.Records.GetRecords() {
			primary := record.Field(mainField.Name())
			if primary.IsValid() == false {
				return errors.New("some records have not primary key")
			}
			rField := LoopIndirect(record.Field(fieldName))
			for subi := 0; subi < rField.Len(); subi++ {
				subRecord := subRecords.GetRecord(subi + offset)
				subPrimary := subRecord.Field(subField.Name())
				if subPrimary.IsValid() == false {
					return errors.New("some records have not primary key")
				}
				middleRecord := NewRecord(middleBrick.model, reflect.New(middleBrick.model.ReflectType).Elem())
				middleRecord.SetField(preload.RelationField.Name(), primary)
				middleRecord.SetField(preload.SubRelationField.Name(), subPrimary)
				middleRecords.Add(middleRecord.Source())
			}
			offset += rField.Len()
		}

		// delete middle model data
		var primaryFields []Field
		if mainSoftDelete == false {
			primaryFields = append(primaryFields, middleBrick.model.GetPrimary()[0])
		}
		if subSoftDelete == false {
			primaryFields = append(primaryFields, middleBrick.model.GetPrimary()[1])
		}
		if len(primaryFields) != 0 {
			conditions := middleBrick.Search
			middleBrick = middleBrick.Conditions(nil)
			for _, primaryField := range primaryFields {
				primarySetType := reflect.MapOf(primaryField.StructField().Type, reflect.TypeOf(struct{}{}))
				primarySet := reflect.MakeMap(primarySetType)
				for _, record := range middleRecords.GetRecords() {
					primarySet.SetMapIndex(record.Field(primaryField.Name()), reflect.ValueOf(struct{}{}))
				}
				var primaryKeys = reflect.New(reflect.SliceOf(primaryField.StructField().Type)).Elem()
				for _, k := range primarySet.MapKeys() {
					primaryKeys = reflect.Append(primaryKeys, k)
				}
				middleBrick = middleBrick.Where(ExprIn, primaryField, primaryKeys.Interface()).
					Or().Conditions(middleBrick.Search)
			}
			middleBrick = middleBrick.And().Conditions(conditions)
			result, err := middleBrick.delete(middleRecords)
			ctx.Result.MiddleModelPreload[fieldName] = result
			if err != nil {
				return err
			}
		}

		result, err := subBrick.deleteWithPrimaryKey(subRecords)
		ctx.Result.Preload[fieldName] = result
		if err != nil {
			return err
		}
	}

	if err := ctx.Next(); err != nil {
		return err
	}

	for fieldName, preload := range ctx.Brick.OneToOnePreload {
		if preload.IsBelongTo == true {
			preloadBrick := ctx.Brick.Preload(fieldName)
			subRecords := MakeRecordsWithElem(preload.SubModel, ctx.Result.Records.GetFieldAddressType(fieldName))
			for _, record := range ctx.Result.Records.GetRecords() {
				subRecords.Add(record.FieldAddress(fieldName))
			}

			mainSoftDelete := preload.Model.GetFieldWithName("DeletedAt") != nil
			subSoftDelete := preload.SubModel.GetFieldWithName("DeletedAt") != nil
			if mainSoftDelete == false && subSoftDelete == true {
				deletedAtField := preloadBrick.model.GetFieldWithName("DeletedAt")
				preloadBrick = preloadBrick.bindDefaultFields(preload.RelationField, deletedAtField)
			}

			result, err := preloadBrick.deleteWithPrimaryKey(subRecords)
			ctx.Result.Preload[fieldName] = result
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func HandlerHardDelete(ctx *Context) error {
	action := ExecAction{}
	action.Exec = ctx.Brick.DeleteExec()
	action.Result, action.Error = ctx.Brick.Exec(action.Exec)
	ctx.Result.AddExecRecord(action)
	return nil
}

//
func HandlerSoftDeleteCheck(ctx *Context) error {
	deletedField := ctx.Brick.model.GetFieldWithName("DeletedAt")
	if deletedField != nil {
		ctx.Brick = ctx.Brick.Where(ExprNull, deletedField).And().Conditions(ctx.Brick.Search)
	}
	return nil
}

func HandlerSoftDelete(ctx *Context) error {
	action := ExecAction{}
	now := time.Now()
	value := reflect.New(ctx.Brick.model.ReflectType).Elem()
	record := NewStructRecord(ctx.Brick.model, value)
	record.SetField("DeletedAt", reflect.ValueOf(now))
	bindFields := []interface{}{"DeletedAt"}
	for _, preload := range ctx.Brick.OneToOnePreload {
		if preload.IsBelongTo {
			subSoftDelete := preload.SubModel.GetFieldWithName("DeletedAt") != nil
			if subSoftDelete == false {
				rField := preload.RelationField
				bindFields = append(bindFields, rField.Name())
				record.SetField(rField.Name(), reflect.Zero(rField.StructField().Type))
			}
		}
	}
	ctx.Brick = ctx.Brick.BindFields(ModeUpdate, bindFields...)
	action.Exec = ctx.Brick.UpdateExec(record)
	action.Result, action.Error = ctx.Brick.Exec(action.Exec)
	ctx.Result.AddExecRecord(action)
	return nil
}
