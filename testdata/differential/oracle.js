"use strict";

const fs = require("fs");
const dagre = require(process.env.DAGRO_DAGRE_JS);
const input = JSON.parse(fs.readFileSync(0, "utf8"));
const options = input.options || {};
const graphOptions = {
  directed: options.directed === undefined ? true : options.directed,
  multigraph: !!options.multigraph,
  compound: !!options.compound,
};
const g = new dagre.graphlib.Graph(graphOptions)
  .setGraph(input.graph || {})
  .setDefaultNodeLabel(() => ({}))
  .setDefaultEdgeLabel(() => ({}));

for (const node of input.nodes || []) {
  g.setNode(node.id, node.attrs || {});
}
for (const node of input.nodes || []) {
  if (Object.prototype.hasOwnProperty.call(node, "parent")) {
    g.setParent(node.id, node.parent);
  }
}
for (const edge of input.edges || []) {
  if (Object.prototype.hasOwnProperty.call(edge, "name")) {
    g.setEdge(edge.v, edge.w, edge.attrs || {}, edge.name);
  } else {
    g.setEdge(edge.v, edge.w, edge.attrs || {});
  }
}

dagre.layout(g);

const output = {
  graph: { width: g.graph().width, height: g.graph().height },
  nodes: g.nodes().map((id) => {
    const n = g.node(id);
    return { id, x: n.x, y: n.y, width: n.width, height: n.height };
  }),
  edges: g.edges().map((edgeObj) => {
    const e = g.edge(edgeObj);
    const out = {
      v: edgeObj.v,
      w: edgeObj.w,
      namePresent: Object.prototype.hasOwnProperty.call(edgeObj, "name"),
      name: edgeObj.name || "",
      points: e.points,
      xPresent: Object.prototype.hasOwnProperty.call(e, "x"),
      yPresent: Object.prototype.hasOwnProperty.call(e, "y"),
    };
    if (out.xPresent) out.x = e.x;
    if (out.yPresent) out.y = e.y;
    return out;
  }),
};

process.stdout.write(JSON.stringify(output));
