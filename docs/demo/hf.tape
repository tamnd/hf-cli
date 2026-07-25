# Demo tape for hf. Rendered with ascii-gif (github.com/tamnd/ascii-gif),
# which supplies the window chrome and theme; this file is just the action.
#
#   ascii-gif render docs/demo/hf.tape -o docs/static/demo.gif
#
# hf must be on PATH inside the recording shell, and the hub reachable.

Hide
Type "export PS1='$ ' PATH=$HOME/bin:$PATH && cd $(mktemp -d) && clear"
Enter
Show

Sleep 800ms
Type "hf model google-bert/bert-base-uncased --fields id,likes,downloads,task"
Sleep 700ms
Enter
Sleep 3.5s

Type "clear"
Enter
Sleep 300ms
Type "hf models --author google --fields id,likes,downloads,task -n 5"
Sleep 700ms
Enter
Sleep 4s

Type "clear"
Enter
Sleep 300ms
Type "hf schema rajpurkar/squad --split train --fields name,dtype,class"
Sleep 700ms
Enter
Sleep 4s

Type "clear"
Enter
Sleep 300ms
Type "hf edges google/gemma-4-31B-it --fields predicate,object,source -n 6"
Sleep 700ms
Enter
Sleep 4s

Type "clear"
Enter
Sleep 300ms
Type "hf rdf google-bert/bert-base-uncased --format ttl | grep -m5 schema:"
Sleep 700ms
Enter
Sleep 4.5s
