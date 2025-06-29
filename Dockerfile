FROM scratch
COPY bandgo /
ENTRYPOINT [ "/bandgo" ]