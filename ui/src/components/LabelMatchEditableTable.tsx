// Copyright 2021 MicroOps
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

import type { ProColumns } from '@ant-design/pro-components';
import { EditableProTable } from '@ant-design/pro-components';
import { EditableProTableProps } from '@ant-design/pro-table/es/components/EditableTable';
import React, { useEffect, useState } from 'react';

export type DataSourceType = {
  id: React.Key;
  name?: string;
  match?: string;
};

interface TableProps<T = DataSourceType> extends EditableProTableProps<T,any>{
  values: DataSourceType[]
  onChange: (value: T[]) => void;
  className: string;
}


export const EditableTable: React.FC<TableProps> = ({values,onChange,className,...props}) => {
  const [editableKeys, setEditableRowKeys] = useState<React.Key[]>([]);

  useEffect(()=>{
    setEditableRowKeys(values.map(item=>item.id))
  },[values])

  const columns: ProColumns<DataSourceType>[] = [
    {
      title: 'Label name',
      dataIndex: 'name',
      fieldProps:{
        placeholder: "Label name",
      },
      width: 200,
    },{
      title: 'Label value match',
      fieldProps:{
        placeholder: "Label value match",
      },
      dataIndex: 'match',
    },
    {
      title: '操作',
      valueType: 'option',
      width: 100,
      render: () => {
        return null;
      },
    },
  ];

  return (
    <>
      <EditableProTable<DataSourceType>
        columns={columns}
        className={className}
        rowKey="id"
        title={()=>"label matching rule"}
        locale={{
          emptyText: ()=>undefined
        }}
        showHeader={false}
        size={"small"}
        value={values}
        onChange={onChange}
        recordCreatorProps={{
          newRecordType: 'dataSource',
          record: () => ({
            id: Date.now(),
          }),
        }}
        editable={{
          type: 'multiple',
          editableKeys,
          actionRender: (row, config, defaultDoms) => {
            return [defaultDoms.delete];
          },
          onValuesChange: (record, recordList) => {
            onChange(recordList);
          },
        }}
        {...props}
      />
    </>
  );
};
export default EditableTable;