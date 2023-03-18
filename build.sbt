enablePlugins(ScalaJSPlugin)

name := "nostr-army-knife"
scalaVersion := "2.13.7"

scalaJSUseMainModuleInitializer := true

libraryDependencies += "com.armanbilge" %%% "calico" % "0.2.0-RC2"
libraryDependencies += "com.fiatjaf" %%% "snow" % "0.2.0-RC2"
