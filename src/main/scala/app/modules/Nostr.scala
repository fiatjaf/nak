package app.modules

import scala.scalajs.js.annotation._
import scala.scalajs.js

@js.native
@JSGlobal
object Nostr extends js.Object {
  def getPublicKey(text: String): String = js.native
  def getEventHash(evt: js.Dynamic): String = js.native
  def serializeEvent(evt: js.Dynamic): String = js.native
  def verifySignature(evt: js.Dynamic): Boolean = js.native
  def signEvent(evt: js.Dynamic, privateKey: String): String = js.native
}
