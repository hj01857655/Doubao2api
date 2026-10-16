import re, sys

html = open('_page.html', encoding='utf-8', errors='replace').read()
i = html.find('static/js/async/')
seg = html[i:]

def bal_obj(start):
    depth = 0
    for k in range(start, len(seg)):
        c = seg[k]
        if c == '{':
            depth += 1
        elif c == '}':
            depth -= 1
            if depth == 0:
                return seg[start:k + 1], k + 1
    return '', len(seg)

j = seg.find('({')
s = j + 1
o1, e1 = bal_obj(s)
nm = dict(re.findall(r'([0-9a-zA-Z_]+):"([^"]+)"', o1))

m = seg.find('+{', e1)
o2, e2 = bal_obj(m + 1)
hs = dict(re.findall(r'([0-9a-zA-Z_]+):"([^"]+)"', o2))

base = 'https://lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/chat/static/js/async'
cid = sys.argv[1]
print('name=%s hash=%s' % (nm.get(cid, '?'), hs.get(cid, '?')))
if hs.get(cid):
    print('%s/%s.%s.js' % (base, nm.get(cid, cid), hs[cid]))
else:
    print('NONE')