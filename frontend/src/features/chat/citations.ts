// remarkCitations turns bracket citations ([1], [2, 5]) in assistant answers
// into one cite:N link per number, which the UI renders as a chip with a
// hover preview (file name + page). It only rewrites plain text nodes —
// inlineCode and code blocks carry raw values, so `[1]` inside backticks is
// never touched. Numbers outside 1..max stay as bracketed plain text,
// matching the backend's citation discipline (only shown brackets are valid).

interface TextNode {
  type: "text";
  value: string;
}

interface LinkNode {
  type: "link";
  url: string;
  children: TextNode[];
}

interface ParentNode {
  type: string;
  children: MdNode[];
}

type MdNode = TextNode | LinkNode | ParentNode;

const CITATION_RE = /\[(\d+(?:\s*,\s*\d+)*)\]/g;

function isParent(node: MdNode): node is ParentNode {
  return "children" in node && Array.isArray((node as ParentNode).children);
}

function splitCitationText(node: TextNode, max: number): MdNode[] {
  const out: MdNode[] = [];
  let last = 0;
  for (let m = CITATION_RE.exec(node.value); m !== null; m = CITATION_RE.exec(node.value)) {
    if (m.index > last) out.push({ type: "text", value: node.value.slice(last, m.index) });
    const nums = m[1].split(",").map((s) => parseInt(s.trim(), 10));
    if (nums.every((n) => n >= 1 && n <= max)) {
      // One chip per number; the UI drops the brackets when every number in
      // the group is valid, so spacing comes from the chip's own margin.
      for (const n of nums) {
        out.push({ type: "link", url: `cite:${n}`, children: [{ type: "text", value: String(n) }] });
      }
    } else {
      out.push({ type: "text", value: m[0] });
    }
    last = m.index + m[0].length;
  }
  if (last < node.value.length) out.push({ type: "text", value: node.value.slice(last) });
  return out;
}

function visit(parent: ParentNode, max: number): void {
  const out: MdNode[] = [];
  for (const child of parent.children) {
    if (child.type === "text") {
      out.push(...splitCitationText(child as TextNode, max));
    } else {
      if (isParent(child)) visit(child, max);
      out.push(child);
    }
  }
  parent.children = out;
}

export function remarkCitations(max: number) {
  return (tree: ParentNode) => {
    visit(tree, max);
  };
}
