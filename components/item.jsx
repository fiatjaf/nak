import React from 'react'

export default function Item({label, children}) {
  return (
    <div
      style={{
        marginBottom: '9px',
        whiteSpace: 'pre-wrap',
        wordWrap: 'break-word',
        wordBreak: 'break-all'
      }}
    >
      <b>{label}: </b>
      {children}
    </div>
  )
}
