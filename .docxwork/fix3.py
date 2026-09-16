import re
for p in [r"D:\VST DEVELOP\aviation-journey-agent\docs\uml\2-1_system_architecture.puml",
          r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\arch2.puml"]:
    lines = open(p, encoding="utf-8").read().splitlines()
    out = []
    for line in lines:
        if out and line.startswith("（经 ") and "SkEta" in out[-1]:
            out[-1] = out[-1].rstrip() + "\\n" + line.strip()
        elif out and line and not line.startswith(("@", "'", "}", "package", "component", "note", "end note", "title", "skinparam")) \
             and out[-1].rstrip().endswith(("坐标", "Input)")) and ":" in out[-1] and ("-->" in out[-1] or "..>" in out[-1]):
            out[-1] = out[-1].rstrip() + "\\n" + line.strip()
        else:
            out.append(line)
    open(p, "w", encoding="utf-8", newline="\n").write("\n".join(out) + "\n")
    print("fixed", p)
