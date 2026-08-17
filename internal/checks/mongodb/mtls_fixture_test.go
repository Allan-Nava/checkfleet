package mongodb

// testClientPEM is a throwaway certificate+key pair used only to prove the
// driver loads client credentials from the URI (CF-183). It is generated for
// the test suite, matches no real system, and is never used to connect
// anywhere: the private key here is deliberately worthless.
const testClientPEM = `-----BEGIN CERTIFICATE-----
MIIBiTCCAS+gAwIBAgIUCLBAXG88sIvq1w1Tcwp4lJfVUbowCgYIKoZIzj0EAwIw
GjEYMBYGA1UEAwwPY2hlY2tmbGVldC10ZXN0MB4XDTI2MDgxNzA4MDcwOVoXDTM2
MDgxNDA4MDcwOVowGjEYMBYGA1UEAwwPY2hlY2tmbGVldC10ZXN0MFkwEwYHKoZI
zj0CAQYIKoZIzj0DAQcDQgAEHwFKi5P4rciIUlVPBNIVzJH9zhjgnQgZdY8PNi4h
9Za7/Nj9bE58hIFPQFIQH9JiN8sSx0QUOdpW0x0kUI1+x6NTMFEwHQYDVR0OBBYE
FB3oghIj+T8Rm1hgOJUWfz5J2Kw/MB8GA1UdIwQYMBaAFB3oghIj+T8Rm1hgOJUW
fz5J2Kw/MA8GA1UdEwEB/wQFMAMBAf8wCgYIKoZIzj0EAwIDSAAwRQIhAKxclTKX
cIuOS9qiJuaQZWnjiLnshfMwmWcy3WWrbZZaAiAZ+a+yykOsqmt/GCLy5WKJlBeH
jveteT1NTuNWdG5dFQ==
-----END CERTIFICATE-----
-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgAZoscL3Az8Lb0IWK
Y+tJ+znWlb5+u1Agjvj9lL2jM/ahRANCAAQfAUqLk/ityIhSVU8E0hXMkf3OGOCd
CBl1jw82LiH1lrv82P1sTnyEgU9AUhAf0mI3yxLHRBQ52lbTHSRQjX7H
-----END PRIVATE KEY-----
`
