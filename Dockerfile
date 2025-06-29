FROM scratch
COPY bandgo /
ENV th="" url="" referer="" opt="" 
ENTRYPOINT /bandgo -c ${th} -s ${url} -r ${referer} ${opt}