src = open(r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\gen_puml.py", encoding="utf-8").read()
out_lines = []
for line in src.splitlines():
    if "component" in line or "-->" in line or ".up.>" in line or "..|>" in line:
        line = line.replace("\\n", "@NL@")
    out_lines.append(line)
patched = "\n".join(out_lines) + "\n"
# 输出时把占位符换回 PlantUML 需要的字面量 \n
patched = patched.replace('open(r"D:\\VST DEVELOP\\aviation-journey-agent\\docs\\uml\\2-1_system_architecture.puml", "w", encoding="utf-8", newline="\\n").write(out)',
                          'open(r"D:\\VST DEVELOP\\aviation-journey-agent\\docs\\uml\\2-1_system_architecture.puml", "w", encoding="utf-8", newline="\\n").write(out.replace("@NL@", "\\\\n"))')
patched = patched.replace('open(r"D:\\VST DEVELOP\\aviation-journey-agent\\.docxwork\\arch2.puml", "w", encoding="utf-8", newline="\\n").write(out)',
                          'open(r"D:\\VST DEVELOP\\aviation-journey-agent\\.docxwork\\arch2.puml", "w", encoding="utf-8", newline="\\n").write(out.replace("@NL@", "\\\\n"))')
open(r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\gen_puml3.py", "w", encoding="utf-8").write(patched)
print("ok")
