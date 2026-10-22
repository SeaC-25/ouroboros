import React, { useState } from 'react';
export function Comp() {
  const [a, setA] = useState([]);
  const [b, setB] = useState([]);
  const [c, setC] = useState([]);
  const [d, setD] = useState([]);
  const [e, setE] = useState([]);
  const [f, setF] = useState([]);
  return <Panel x={a} y={b} z={c} q={d} r={e} s={f} />;
}
function Panel({ x, y, z, q, r, s }) {
  return <div>{x}{y}{z}{q}{r}{s}</div>;
}
