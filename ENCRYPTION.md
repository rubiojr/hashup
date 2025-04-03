# A note about age encryption in HashUp

The age encryption used when encrypting NATS messages isn't a suitable encryption method, as it lacks some desired properties for message exchange:

> Indeed, we don't want people to use age for messaging, because it would be a pretty lousy messaging encryption tool: no forward secrecy, no ratcheting, no authentication... age is optimized for file encryption, so the occasional reminder of that is something we want to keep, although I agree PAYLOAD would be a good generic word.

From age author in https://github.com/FiloSottile/age/discussions/236#discussioncomment-628046

With that in mind, NATS can use [mutual TLS](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/tls_mutual_auth) on top of the message payload encryption HashUp does with Age, which is probably good enough when both are combined.
