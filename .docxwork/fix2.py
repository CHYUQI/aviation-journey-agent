import io
src = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\arch.puml"
dst = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\arch2.puml"
txt = open(src, encoding="utf-8-sig").read()
lines = txt.splitlines()
out = []
i = 0
while i < len(lines):
    l = lines[i]
    if l.strip().startswith("/'"):
        body = l.strip()[2:]
        if body.endswith("'/") and len(body) > 3:
            out.append("'" + body[:-2].strip())
            i += 1
            continue
        out.append("'" + body.strip())
        i += 1
        while i < len(lines):
            cur = lines[i]
            if cur.strip().endswith("'/"):
                out.append("'" + cur.strip()[:-2].strip())
                i += 1
                break
            out.append("'" + cur.strip())
            i += 1
        continue
    out.append(l)
    i += 1
open(dst, "w", encoding="utf-8", newline="\n").write("\n".join(out) + "\n")
print("\n".join(out[:8]))
