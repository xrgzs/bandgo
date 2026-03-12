FROM scratch
COPY bandgo /
ENTRYPOINT [ "/bandgo" ]
CMD [ "-q" ]
